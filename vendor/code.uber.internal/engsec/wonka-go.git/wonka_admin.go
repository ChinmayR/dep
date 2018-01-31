package wonka

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"go.uber.org/zap"
	"golang.org/x/crypto/ssh"
)

func (w *uberWonka) Admin(ctx context.Context, req AdminRequest) (err error) {
	m := w.metrics.Tagged(map[string]string{"endpoint": "admin"})
	stopWatch := m.Timer("time").Start()
	defer stopWatch.Stop()
	m.Counter("call").Inc(1)
	defer func() {
		name := "success"
		if err != nil {
			// TODO(jkline): Differentiate between 400ish client side failures
			// and 500ish server side failures. Currently we don't get back the
			// http response object so there is no firm way to tell.
			name = "failure"
		}
		m.Counter(name).Inc(1)
	}()

	if w.ussh == nil {
		return errors.New("no ussh cert")
	}

	req.Ctime = time.Now().Unix()
	req.Ussh = string(ssh.MarshalAuthorizedKey(w.ussh))
	toSign, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("marsalling admin request: %v", err)
	}

	sig, err := w.sshSignMessage(toSign)
	if err != nil {
		return fmt.Errorf("signing ssh message: %v", err)
	}
	req.Signature = base64.StdEncoding.EncodeToString(sig.Blob)
	req.SignatureFormat = sig.Format

	switch req.Action {
	case DeleteEntity:
		w.log.Debug("request to delete",
			zap.Any("entity_to_delete", req.EntityName),
		)

		var resp GenericResponse
		if err := w.httpRequester.Do(ctx, adminEndpoint, req, &resp); err != nil {
			if resp.Result != "" {
				err = fmt.Errorf("%s", resp.Result)
			}
			w.log.Error("https request error",
				zap.Error(err),
				zap.Any("action", req.Action),
				zap.Any("entity_to_delete", req.EntityName),
			)

			return err
		}
		w.log.Debug("response", zap.Any("response", resp.Result))

	default:
		return fmt.Errorf("invalid admin action: %s", req.Action)
	}

	return nil
}
