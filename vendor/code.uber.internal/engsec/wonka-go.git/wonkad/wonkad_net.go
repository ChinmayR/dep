package main

import (
	"context"
	"crypto/ecdsa"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"

	wonka "code.uber.internal/engsec/wonka-go.git"
	"code.uber.internal/engsec/wonka-go.git/wonkacrypter"

	"go.uber.org/zap"
)

// verifySignature verifies the signature on a certificate refresh message
func verifySignature(log *zap.Logger, req wonka.WonkadRequest) bool {
	// verify the cert is good.
	c := &req.Certificate
	if err := c.CheckCertificate(); err != nil {
		log.Error("certificate is invalid", zap.Error(err))
		return false
	}

	// now verify that the cert private key signed this request.
	toVerifyReq := req
	toVerifyReq.Signature = nil
	toVerify, err := json.Marshal(toVerifyReq)
	if err != nil {
		log.Error("json marshal", zap.Error(err))
		return false
	}

	k, err := c.PublicKey()
	if err != nil {
		log.Error("couldn't get public key")
		return false
	}

	return wonkacrypter.New().Verify(toVerify, req.Signature, k)
}

func requestClaim(ctx context.Context, w wonka.Wonka, req wonka.WonkadRequest) (*wonka.Claim, error) {
	clm, err := w.ClaimRequest(ctx, w.EntityName(), req.Destination)
	return clm, fmt.Errorf("error requesting claim: %v", err)
}

func (w *wonkad) handleWonkaRequest(ctx context.Context, req wonka.WonkadRequest) (*wonka.Certificate, *ecdsa.PrivateKey, *wonka.Claim, error) {
	// TODO(pmoody): find a better way to do this.
	// request a claim for some process running on the host, or ask wonkamaster
	// if this task+taskid should be running on this host.
	if req.Destination != "" {
		w.log.Info("trying to get a claim",
			zap.String("hostname", w.host),
			zap.String("service", req.Service),
		)
		claim, err := requestClaim(ctx, w.wonka, req)
		if err != nil {
			w.log.Error("error requesting claim", zap.Error(err))
			return nil, nil, nil, fmt.Errorf("marshalling claim: %v", err)
		}
		return nil, nil, claim, nil
	} else if req.Service != "" {
		if req.TaskID == "" {
			w.log.Error("no task id")
			// ugly hack to account for cases where a process does not provide a task id
			req.TaskID = fmt.Sprintf("%s-%s", w.host, req.Service)
		}

		// fill out the certificate
		cert, privKey, err := wonka.NewCertificate(
			wonka.CertEntityName(req.Service),
			wonka.CertHostname(w.host),
			wonka.CertTaskIDTag(req.TaskID))
		if err != nil {
			w.log.Error("error generating certificate", zap.Error(err))
			return nil, nil, nil, fmt.Errorf("error generating cert: %v", err)
		}

		if err := w.wonka.CertificateSignRequest(ctx, cert, nil); err != nil {
			w.log.Error("error requesting certificate", zap.Error(err))
			return nil, nil, nil, fmt.Errorf("error requesting cert: %v", err)
		}

		return cert, privKey, nil, nil
	}

	return nil, nil, nil, errors.New("nothing requested nothing gained")
}

func writeCertAndKey(conn net.Conn, cert *wonka.Certificate, privKey *ecdsa.PrivateKey) error {
	certBytes, err := wonka.MarshalCertificate(*cert)
	if err != nil {
		return fmt.Errorf("error marshalling certificate: %v", err)
	}
	privKeyBytes, err := x509.MarshalECPrivateKey(privKey)
	if err != nil {
		return fmt.Errorf("error marshalling private key: %v", err)
	}

	repl := wonka.WonkadReply{
		Certificate: certBytes,
		PrivateKey:  privKeyBytes,
	}

	toWrite, err := json.Marshal(repl)
	if err != nil {
		return fmt.Errorf("error marshalling reply: %v", err)
	}

	_, err = conn.Write(toWrite)
	if err != nil {
		return fmt.Errorf("error writing certificate: %v", err)
	}
	return nil
}

func (w *wonkad) serve(conn net.Conn, needSig bool) {
	w.log.Debug("calling serve")
	defer conn.Close()

	var req wonka.WonkadRequest
	if err := json.NewDecoder(conn).Decode(&req); err != nil {
		w.log.Error("decoding request", zap.Error(err))
		return
	}

	if needSig && !verifySignature(w.log, req) {
		conn.Write([]byte(fmt.Sprintf("no sig, no claim, %s", req.Process)))
		return
	}

	w.log.Debug("handling request")
	cert, privKey, claim, err := w.handleWonkaRequest(context.Background(), req)
	if err != nil {
		w.log.Error("error handling request", zap.Error(err))
		conn.Write([]byte(err.Error()))
		return
	}

	if cert != nil {
		err := writeCertAndKey(conn, cert, privKey)
		if err != nil {
			w.log.Error("", zap.Error(err))
			return
		}
	}

	if claim != nil {
		w.log.Debug("good request", zap.Any("claim", claim))
		toWrite, err := wonka.MarshalClaim(claim)
		if err != nil {
			conn.Write([]byte(err.Error()))
			return
		}
		conn.Write([]byte(toWrite))
		return
	}

}

func (w *wonkad) listen(l net.Listener) {
	// check signature when mesos and docker support bootstrapping.
	for {
		c, err := l.Accept()
		if err != nil {
			continue
		}

		w.log.Info("new connection", zap.Any("address", c.RemoteAddr()))
		go w.serve(c, false)
	}
}

func (w *wonkad) listenAndServe() error {
	go w.listen(w.unixListener)
	go w.listen(w.tcpListener)

	// now, we wait
	select {}
}

func (w *wonkad) setupListeners(u, t string) error {
	w.log.Debug("removing path", zap.Any("path", u))
	if err := os.Remove(u); err != nil {
		w.log.Info("removing path, continuing", zap.Error(err))
	}

	var err error
	w.log.Debug("setting up unix listener")
	w.unixListener, err = net.Listen("unix", u)
	if err != nil {
		return fmt.Errorf("setting up unix listener: %v", err)
	}

	unixL, ok := w.unixListener.(*net.UnixListener)
	if !ok {
		return errors.New("not a unix listener !?")
	}

	unixFile, err := unixL.File()
	if err != nil {
		return fmt.Errorf("getting underlying file: %v", err)
	}

	w.log.Debug("setting mode on unix listener")
	if err := unixFile.Chmod(os.FileMode(0777)); err != nil {
		// this is dumb and racey but I'm getting invalid argument errors
		// when I try chmod the file handle.
		if err := os.Chmod(u, os.FileMode(0666)); err != nil {
			return fmt.Errorf("setting mode on unix listener: %v", err)
		}
	}

	w.log.Debug("setting up tcp listener", zap.Any("port", t))
	w.tcpListener, err = net.Listen("tcp", t)
	if err != nil {
		return fmt.Errorf("setting up tcp listener: %v", err)
	}

	w.log.Debug("listeners successfully setup")
	return nil
}
