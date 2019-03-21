package main

import (
	"fmt"

	"code.uber.internal/engsec/usso-cli.git/ussotoken"
)

// this is sample code shows how to use ussocli package to get offline token via ussh certs
func usshUSSO(domain string) int {
	// display uSSO CLI tool banner (optional)
	displayBanner()
	// create a ussh object based on ussh cert
	ussh, err := ussotoken.NewUSSH()
	if err != nil {
		fmt.Println("Error: " + err.Error())
		return 1
	}
	offineTokenRes, err := ussh.GetOfflineToken(domain)
	if err != nil {
		fmt.Println("Error: " + err.Error())
		return 1
	}
	fmt.Println(offineTokenRes.Token)
	return 0
}
