package main

import (
	"flag"
	"fmt"
	"os"
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
)

// Main entry point for uSSO client tool
func main() {
	// Print uSSO client version
	version := flag.Bool("version", false, "uSSO client tool verson")
	// -login, this flag enable users to get a uSSO offline token using their Onelogin credentials and Duo 2FA
	login := flag.String("login", "", `Login via uSSO cli tool and get a uSSO offline token.
	Please specify the domain you would like to access
	e.g. usso -login whober.uberinternal.com`)
	// -ussh, this flag enable users to get a uSSO offline token using their existing ussh cert
	ussh := flag.String("ussh", "", `Exchange ussh cert to get a uSSO offline token.
	Please specify the domain you would like to access
	e.g. usso -ussh whober.uberinternal.com`)
	flag.Parse()
	if *version {
		fmt.Println("uSSO client version: " + VERSION)
	} else if *login != "" {
		loginUSSO(*login)
		os.Exit(0)
	} else if *ussh != "" {
		usshUSSO(*ussh)
		os.Exit(0)
	} else {
		fmt.Println("Please try usso -help for instruction")
		os.Exit(0)
	}
}
