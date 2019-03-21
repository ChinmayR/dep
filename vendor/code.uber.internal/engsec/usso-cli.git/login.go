package main

import (
	"fmt"

	"code.uber.internal/engsec/usso-cli.git/ussotoken"
)

// this is sample code shows how to use ussocli package to get offline token via MFA
func loginUSSO(domain string) int {
	// display uSSO CLI tool banner (optional)
	displayBanner()
	offineTokenRes, err := ussotoken.NewLogin(domain)
	if err != nil {
		fmt.Println("Error: " + err.Error())
		return 1
	}
	fmt.Println(offineTokenRes.Token)
	return 0
}
