package ussotoken

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"golang.org/x/crypto/ssh/terminal"
)

// Define constant variables here
const (
	VERSION             = "1.0"
	SupportedDomain     = "uberinternal.com"
	EnvUberOwner        = "UBER_OWNER"
	LoginURLEndpoint    = "/oidauth/login_cli"
	MfaURLEndpoint      = "/oidauth/mfa_cli"
	PasscodeURLEndpoint = "/oidauth/passcode_cli"
	UsshEndpoint        = "/oidauth/offline_cli"
	CaRootPubKey        = "/etc/ssh/trusted_user_ca"
	UsshReqExp          = 5 * time.Minute
)

// USSOLoginResponse defines the response from login endpoint
type USSOLoginResponse struct {
	Version    string `json:"version"`
	Type       string `json:"type"`
	Message    string `json:"message"`
	Error      bool   `json:"error"`
	Email      string `json:"email"`
	StateToken string `json:"state_token"`
	DeviceID   int    `json:"device_id"`
}

// USSOCliResponse defines the response from MFA / passcode verification endpoint
type USSOCliResponse struct {
	Version string `json:"version"`
	Type    string `json:"type"`
	Message string `json:"message"`
	Error   bool   `json:"error"`
	Email   string `json:"email"`
	Token   string `json:"token"`
}

//******************
// Login
//******************

// loginUser defines the request parameters for login endpoint
type loginUser struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// mfaUser defines the request parameters for MFA endpoint
type mfaUser struct {
	Factor     string `json:"factor"`
	Username   string `json:"username"`
	StateToken string `json:"state_token"`
	DeviceID   string `json:"device_id"`
}

// passcodeUser defines the request parameters for passcode verification endpoint
type passcodeUser struct {
	Username string `json:"username"`
	Passcode string `json:"passcode"`
	Factor   string `json:"factor"`
}

//******************
// uSSH
//******************

// UsshVerifier defines ussh struct
type UsshVerifier struct {
	Email    string
	Cert     *ssh.Certificate
	CaPubKey ssh.PublicKey
}

// USSHRequest defines the USSO request parameters
type USSHRequest struct {
	Version  string `json:"version"`
	Email    string `json:"email"`
	Expire   int64  `json:"expire"`
	USSHCert []byte `json:"usshcert"`
}

// USSHRequestWithSig combines USSORequest and USSOSignature together
type USSHRequestWithSig struct {
	Request   string `json:"USSHRequest"`
	Signature string `json:"USSHSignature"`
}

// USSHResponse defines the USSO response parameters
type USSHResponse struct {
	Version string `json:"version"`
	Message string `json:"message"`
	Error   bool   `json:"error"`
	Token   string `json:"token"`
}

// Ussh defines ussh struct
type Ussh struct {
	Email  string
	Cert   *ssh.Certificate
	Signer ssh.Signer
}

// NewUSSH generates a new Ussh stuct
func NewUSSH() (*Ussh, error) {
	email := os.Getenv(EnvUberOwner)
	strsEmail := strings.Split(email, "@")
	if len(strsEmail) < 2 {
		return nil, errors.New("env variable " + EnvUberOwner + " is not set, please ensure you are using a Uber managed machine")
	}
	authSock := os.Getenv("SSH_AUTH_SOCK")
	if len(authSock) == 0 {
		return nil, errors.New("Env SSH_AUTH_SOCK is not set")
	}
	agentSock, err := net.Dial("unix", authSock)
	if err != nil {
		return nil, err
	}
	sshAgent := agent.NewClient(agentSock)
	signers, err := sshAgent.Signers()
	if err != nil {
		return nil, err
	}
	if len(signers) == 0 {
		return nil, errors.New("no ssh cert is found, please run ussh first")
	}
	for _, signer := range signers {
		cert, err := parseSSHCert(signer.PublicKey().Marshal())
		if err != nil {
			continue
		}
		_, err = VerifySSHCert(cert)
		if err != nil {
			return nil, err
		}
		if strsEmail[0] != cert.ValidPrincipals[0] {
			return nil, errors.New("your email name " + strsEmail[0] + " doesn't match your cert principle " + cert.ValidPrincipals[0])
		}
		return &Ussh{Signer: signer, Cert: cert, Email: email}, nil
	}
	return nil, errors.New("no ssh cert is found, please run ussh first")
}

// GetOfflineToken makes a POST reuqest to <service>/oidauth/offline_cli to get offline token
func (u *Ussh) GetOfflineToken(domain string) (*USSOCliResponse, error) {
	hostname := generateHostname(domain)
	serviceURL := hostname + UsshEndpoint
	reqBytes, err := u.GenerateUsshReq()
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequest("POST", serviceURL, bytes.NewBuffer(reqBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create a http request: %v", err)
	}
	httpReq.Header.Set("Content-Type", "application/json; charset=utf-8")
	client := &http.Client{}
	httpResp, err := client.Do(httpReq)
	if err != nil {
		return nil, errors.New("failed to get response from USSO server")
	}
	usshResp := &USSOCliResponse{}
	if json.NewDecoder(httpResp.Body).Decode(usshResp) != nil {
		return nil, fmt.Errorf("failed to json unmarshar ussh response body: %v", err)
	}
	if httpResp.Body.Close() != nil {
		return nil, fmt.Errorf("failed to close http response body io: %v", err)
	}
	return usshResp, nil
}

// GenerateUsshReq generates ussh request
func (u *Ussh) GenerateUsshReq() ([]byte, error) {
	now := time.Now()
	usshReq := &USSHRequest{
		Version:  VERSION,
		Email:    u.Email,
		Expire:   now.Add(UsshReqExp).Unix(),
		USSHCert: u.Cert.Marshal(),
	}
	usshReqEncoded, err := encodeRequest(usshReq)
	if err != nil {
		return nil, err
	}
	sigEncoded, err := u.Sign(usshReqEncoded)
	if err != nil {
		return nil, err
	}
	usshSignedReq := &USSHRequestWithSig{
		Request:   usshReqEncoded,
		Signature: sigEncoded,
	}
	usshSignedReqBytes, err := json.Marshal(usshSignedReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal ussh signed request: %v", err)
	}
	return usshSignedReqBytes, nil
}

// Sign signs ussh request
func (u *Ussh) Sign(usshReqEncoded string) (string, error) {
	sig, err := u.Signer.Sign(rand.Reader, []byte(usshReqEncoded))
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(ssh.Marshal(sig)), nil
}

// encodeRequest encodes HTTP request body
func encodeRequest(request *USSHRequest) (string, error) {
	toSignBytes, err := json.Marshal(*request)
	if err != nil {
		return "", fmt.Errorf("failed to marshal uSSH request data: %v", err)
	}
	return base64.StdEncoding.EncodeToString(toSignBytes), nil
}

// IsSSHCertExpired checks if ssh cert expires
func IsSSHCertExpired(cert *ssh.Certificate) bool {
	return uint64(time.Now().Unix()) > cert.ValidBefore
}

// VerifySSHCert verifies if ssh cert is valid
func VerifySSHCert(cert *ssh.Certificate) (bool, error) {
	if IsSSHCertExpired(cert) == true {
		return false, errors.New("your ssh cert is expired, please run ussh again")
	}
	if cert.CertType != ssh.UserCert {
		return false, errors.New("invalid cert, please clean up your ssh agent and run ussh again")
	}
	return true, nil
}

// parseSSHCert parses ssh cert
func parseSSHCert(signerByte []byte) (*ssh.Certificate, error) {
	pubkey, err := ssh.ParsePublicKey(signerByte)
	if err != nil {
		return nil, err
	}
	cert, ok := pubkey.(*ssh.Certificate)
	if !ok {
		return nil, errors.New("not a valid ssh cert")
	}
	return cert, nil
}

// loadCAPublicKey loads ssh trusted user CA public keys
func loadCAPublicKey() ([]ssh.PublicKey, error) {
	pubKeyBytes, err := ioutil.ReadFile(CaRootPubKey)
	if err != nil {
		return nil, fmt.Errorf("failed to load ssh CA public key file %s: %v", CaRootPubKey, err)
	}
	var pubKeys []ssh.PublicKey
	in := pubKeyBytes
	for {
		pubKey, _, _, rest, err := ssh.ParseAuthorizedKey(in)
		if err != nil {
			return nil, fmt.Errorf("failed to parse ssh CA public key: %v", err)
		}
		pubKeys = append(pubKeys, pubKey)
		if len(rest) == 0 {
			break
		}
		in = rest
	}
	return pubKeys, nil
}

// NewUSSHVerifer creates a ussh verifier
func NewUSSHVerifer(cert []byte, email string) (*UsshVerifier, error) {
	// check if ssh cert can be parsed
	pubKey, err := ssh.ParsePublicKey(cert)
	if err != nil {
		return nil, fmt.Errorf("error parsing publickey: %v", err)
	}
	sshCert, ok := pubKey.(*ssh.Certificate)
	if !ok {
		return nil, errors.New("publickey is not an ssh certificate")
	}
	// validate ssh cert is not expired
	if IsSSHCertExpired(sshCert) {
		return nil, errors.New("ussh cert is expired")
	}
	usshVerifier := &UsshVerifier{Cert: sshCert}
	// validate ssh cert via root CA public key
	caPubKeys, err := loadCA()
	if err != nil {
		return nil, fmt.Errorf("loadCA failed: %v", err)
	}
	for _, k := range caPubKeys {
		if bytes.Equal(sshCert.SignatureKey.Marshal(), k.Marshal()) {
			usshVerifier.CaPubKey = k
		}
	}
	if err := validateCert(usshVerifier.Cert, usshVerifier.CaPubKey); err != nil {
		return nil, err
	}
	// validate email equals to cert principle
	strsEmail := strings.Split(email, "@")
	if strsEmail[0] != sshCert.ValidPrincipals[0] {
		return nil, errors.New("Email " + strsEmail[0] + " doesn't match cert principle " + sshCert.ValidPrincipals[0])
	}
	usshVerifier.Email = email
	return usshVerifier, nil
}

// validateCert validates ssh cert using root CA
func validateCert(cert *ssh.Certificate, caPubKey ssh.PublicKey) error {
	c2 := *cert
	c2.Signature = nil
	body := c2.Marshal()
	// Drop trailing signature length.
	body = body[:len(body)-4]
	return caPubKey.Verify(body, cert.Signature)
}

// loadCA loads local ROOT CA public keys
func loadCA() ([]ssh.PublicKey, error) {
	pubKeyBytes, err := ioutil.ReadFile(CaRootPubKey)
	if err != nil {
		return nil, fmt.Errorf("error opening public key file %s: %v", CaRootPubKey, err)
	}
	var pubKeys []ssh.PublicKey
	in := pubKeyBytes
	for {
		pubKey, _, _, rest, err := ssh.ParseAuthorizedKey(in)
		if err != nil {
			return nil, fmt.Errorf("error parsing ca public key: %v", err)
		}
		pubKeys = append(pubKeys, pubKey)
		if len(rest) == 0 {
			break
		}
		in = rest
	}
	return pubKeys, nil
}

// DecodeUSSHRequest defines a function to decode ussh request
func DecodeUSSHRequest(reqStr string) (*USSHRequest, error) {
	reqDecoded, err := base64.StdEncoding.DecodeString(reqStr)
	if err != nil {
		return nil, fmt.Errorf("failed to do base64 decoding: %v", err)
	}
	ussoRequest := &USSHRequest{}
	if json.Unmarshal(reqDecoded, ussoRequest) != nil {
		return nil, fmt.Errorf("failed to do json unmarshal: %v", err)
	}
	return ussoRequest, nil
}

// generateHostname generates full FQDN based on sub domain name
func generateHostname(domain string) string {
	hostname := domain
	if !strings.HasPrefix(domain, "http") {
		hostname = "https://" + domain
	}
	if !strings.HasSuffix(domain, ".uberinternal.com") {
		hostname = hostname + ".uberinternal.com"
	}
	return hostname
}

// NewLogin generates offline token via MFA
func NewLogin(domain string) (*USSOCliResponse, error) {
	hostname := generateHostname(domain)
	username := os.Getenv(EnvUberOwner)
	// user login
	ussoLoginResJSON, err := cliLogin(hostname, username)
	if err != nil {
		return nil, err
	}
	// collect user's selection on MFA
	factor, err := cliSelectFactor(username)
	if err != nil {
		return nil, err
	}
	// trigger user's selection on MFA
	ussoMfaResJSON, err := cliTriggerFactor(hostname, factor, *ussoLoginResJSON)
	if err != nil {
		return nil, err
	}
	if ussoMfaResJSON.Type == "push" && ussoMfaResJSON.Error == false && ussoMfaResJSON.Token != "" {
		return ussoMfaResJSON, nil
	} else if ussoMfaResJSON.Type == "sms" && ussoMfaResJSON.Error == false {
		// if "SMS passcode" option is selected, we need to verify the passcode
		// get passcode
		ussoPasscodeResJSON, err := cliVerifyPasscode(hostname, username, factor)
		if err != nil {
			return nil, err
		}
		if ussoPasscodeResJSON.Type == "passcode" && ussoPasscodeResJSON.Error == false && ussoPasscodeResJSON.Token != "" {
			return ussoPasscodeResJSON, nil
		}
	} else {
		return nil, err
	}
	return nil, err
}

// cliLogin verifies user's username and password
func cliLogin(hostname string, username string) (*USSOLoginResponse, error) {
	count := 0
	client := &http.Client{}
	loginurl := hostname + LoginURLEndpoint
	// user can try up to 5 times before uSSO client exits
	for count < 5 {
		password := getPassword()
		// add form data
		form := url.Values{}
		form.Add("Username", username)
		form.Add("Password", password)
		req, err := http.NewRequest("POST", loginurl, strings.NewReader(form.Encode()))
		req.PostForm = form
		req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
		ussoLoginResRaw, err := client.Do(req)
		// parse response
		ussoLoginResJSON := &USSOLoginResponse{}
		if ussoLoginResRaw.StatusCode != http.StatusOK && ussoLoginResRaw.StatusCode != http.StatusUnauthorized {
			return nil, errors.New("server side error, error code: " + strconv.Itoa(ussoLoginResRaw.StatusCode))
		}
		err = json.NewDecoder(ussoLoginResRaw.Body).Decode(&ussoLoginResJSON)
		if err != nil {
			return nil, err
		}
		if ussoLoginResRaw.Body.Close() != nil {
			return nil, errors.New("client side error: ussoLoginResRaw.Body.Close()")
		}
		if ussoLoginResJSON.Error == true && strings.ToLower(ussoLoginResJSON.Type) == "unauthorized" {
			fmt.Print("\n" + ussoLoginResJSON.Message)
			count++
			if count == 4 {
				return nil, errors.New("You have tried too many times")
			}
			continue
		} else if ussoLoginResJSON.Error == false && strings.ToLower(ussoLoginResJSON.Type) == "success" {
			return ussoLoginResJSON, nil
		} else {
			return nil, errors.New(ussoLoginResJSON.Message)
		}
	}
	return nil, errors.New("You have tried too many times")
}

// cliSelectFactor defines a function to collect user's choice of 2FA
func cliSelectFactor(username string) (string, error) {
	fmt.Println("\nDuo two-factor login for " + username)
	count := 0
	for count < 5 {
		selection := getFactor()
		if selection == "1" {
			return "push", nil
		} else if selection == "2" {
			return "sms", nil
		} else {
			fmt.Print("\nInvalid selection... Please enter 1 or 2")
			count++
			if count == 4 {
				return "", errors.New("You have tried too many times")
			}
			continue
		}
	}
	return "", errors.New("You have tried too many times")
}

// cliTriggerFactor defines a function to trigger user's choice of 2FA
func cliTriggerFactor(hostname string, factor string, ussoLoginResJSON USSOLoginResponse) (*USSOCliResponse, error) {
	client := &http.Client{}
	mfaurl := hostname + MfaURLEndpoint
	deviceID := strconv.Itoa(ussoLoginResJSON.DeviceID)
	// add form data
	form := url.Values{}
	form.Add("Factor", factor)
	form.Add("Username", ussoLoginResJSON.Email)
	form.Add("DeviceID", deviceID)
	form.Add("StateToken", ussoLoginResJSON.StateToken)
	req, err := http.NewRequest("POST", mfaurl, strings.NewReader(form.Encode()))
	req.PostForm = form
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	// parse response
	ussoMfaResRaw, err := client.Do(req)
	ussoMfaResJSON := &USSOCliResponse{}
	err = json.NewDecoder(ussoMfaResRaw.Body).Decode(&ussoMfaResJSON)
	if err != nil {
		return nil, err
	}
	if ussoMfaResRaw.Body.Close() != nil {
		return nil, errors.New("client side error: ussoMfaResRaw.Body.Close()")
	}
	return ussoMfaResJSON, nil
}

// cliVerifyPasscode defines a function to verify user's passcode received by SMS
func cliVerifyPasscode(hostname string, username string, factor string) (*USSOCliResponse, error) {
	client := &http.Client{}
	verifyurl := hostname + PasscodeURLEndpoint
	count := 0
	passcode := ""
	// user can try up to 5 times before uSSO client exits
	for count < 5 {
		passcode = getPasscode()
		// add form data
		form := url.Values{}
		form.Add("Username", username)
		form.Add("Passcode", passcode)
		form.Add("Factor", "passcode")
		req, err := http.NewRequest("POST", verifyurl, strings.NewReader(form.Encode()))
		req.PostForm = form
		req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
		// parse response
		ussoPasscodeResRaw, err := client.Do(req)
		if err != nil {
			return nil, err
		}
		if ussoPasscodeResRaw.StatusCode != http.StatusOK && ussoPasscodeResRaw.StatusCode != http.StatusUnauthorized {
			return nil, errors.New("server side error, error code: " + strconv.Itoa(ussoPasscodeResRaw.StatusCode))
		}
		ussoPasscodeResJSON := &USSOCliResponse{}
		err = json.NewDecoder(ussoPasscodeResRaw.Body).Decode(&ussoPasscodeResJSON)
		if err != nil {
			return nil, err
		}
		if ussoPasscodeResRaw.Body.Close() != nil {
			return nil, errors.New("client side error: ussoPasscodeResRaw.Body.Close()")
		}
		if ussoPasscodeResJSON.Error == true && strings.ToLower(ussoPasscodeResJSON.Type) == "deny" {
			fmt.Print("\n" + ussoPasscodeResJSON.Message)
			count++
			if count == 4 {
				return nil, errors.New("You have tried too many times")
			}
			continue
		} else if ussoPasscodeResJSON.Error == false && strings.ToLower(ussoPasscodeResJSON.Type) == "passcode" {
			return ussoPasscodeResJSON, nil
		} else {
			return nil, errors.New(ussoPasscodeResJSON.Message)
		}
	}
	return nil, errors.New("You have tried too many times")
}

// getPassword defines a function to collect user's password
func getPassword() string {
	fmt.Print("\nEnter your One-Login password: ")
	bytePassword, err := terminal.ReadPassword(int(syscall.Stdin))
	if err != nil {
		fmt.Println("\nerror: " + err.Error())
	}
	password := string(bytePassword)
	return strings.TrimSpace(password)
}

// getFactor defines a function to collect user's choice of authN factor
// user needs to enter 1 or 2 to make a selection
func getFactor() string {
	fmt.Println("\nSelect one of the following options: ")
	fmt.Print("\n 1. Push notification via Duo app ")
	fmt.Println("\n 2. Passcode via SMS message ")
	reader := bufio.NewReader(os.Stdin)
	fmt.Print("\nChoose option (1-2): ")
	byteInput, err := reader.ReadString('\n')
	if err != nil {
		fmt.Println("\nError: " + err.Error())
	}
	factor := string(byteInput)
	return strings.TrimSpace(factor)
}

// getPasscode defines a function to collect user's passcode received by SMS
func getPasscode() string {
	reader := bufio.NewReader(os.Stdin)
	fmt.Print("\nEnter the 7 digits passcode from SMS: ")
	byteInput, err := reader.ReadString('\n')
	if err != nil {
		fmt.Println("\nerror: " + err.Error())
	}
	passcode := string(byteInput)
	return strings.TrimSpace(passcode)
}

// saveToken defines a function to save offline token in a file where other CLI tools can read from
// For example, if an user get offline token from test_foo.uberinternal.com
// The token will be set as /tmp/test_foo
func saveToken(hostname, token string) error {
	strs := strings.Split(hostname, "://")
	subdomain := strings.Split(strs[1], ".")[0]
	data := []byte(token)
	return ioutil.WriteFile("/tmp/"+subdomain, data, 0644)
}
