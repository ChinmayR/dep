package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"strings"
)

// displayBanner displays uSSO cli tool banner
func displayBanner() {
	fmt.Print("\n+-+-+-+-+ +-+-+-+ +-+-+-+-+")
	fmt.Print("\n|u|s|s|o| |c|l|i| |t|o|o|l|")
	fmt.Print("\n+-+-+-+-+ +-+-+-+ +-+-+-+-+\n")
}

// setSysVariableName sets an enviroment variable by name
func setSysVariableName(url, token string) error {
	strs := strings.Split(url, "://")
	subdomain := strings.Split(strs[1], ".")[0]
	sysVar := strings.ToUpper(subdomain + "_OFFLINE_TOKEN")
	return os.Setenv(sysVar, token)
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
