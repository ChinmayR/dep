# uSSO CLI tool

When developers interact with `*.uberinternal.com` using their CLI tools, they would like to have a programmatical way of retrieving offline token. uSSO CLI is a tool to allow developers to achieve that seamless authentication experience.

There are two options you can use uSSO cli tool:
  - Use it as a stand alone binary
  - Import it as a Go package

Either option provides two ways of retrieving an uSSO offline token:
  - Using existing uSSH cert
  - Authenticate with Onelogin and Duo

If you choose to use the cli tool as stand alone binary, you can build the tool (`homebrew` is coming soon) from source code by:

```
$ cd ~/gocode/src/code.uber.internal/engsec
$ git clone gitolite@code.uber.internal:engsec/usso-cli usso-cli.git
$ make
```
The `usso` binary will be generated.

### How to use uSSO CLI tool as a stand alone binary
If you have a valid uSSH cert, you can use existing uSSH cert to get a offline token:
```
$ ./usso -ussh usso-test-site

+-+-+-+-+ +-+-+-+ +-+-+-+-+
|u|s|s|o| |c|l|i| |t|o|o|l|
+-+-+-+-+ +-+-+-+ +-+-+-+-+

Your offline token for https://usso-test-site.uberinternal.com is:

eyJhbGciOiJFUzI1NiIsImVudiI6InByb2QiLCJraWQiOiJwdWItdXNzby1zdmMtMDkxNzE4LnBlbSIsInR5cCI6IkpXVCIsInZlciI6IjEuMCJ9.eyJjbGllbnRfaWQiOiJ1c3NvLXRlc3Qtc2l0ZS51YmVyaW50ZXJuYWwuY29tIiwiZW1haWwiOiJnYmFvQHViZXIuY29tIiwiZXhwIjoxNTUwNjA5NzQyLCJpYXQiOjE1NTA1Mzc0NDIsImlzcyI6InNwaWZmZTovL2NvbXB1dGUuaG9zdHMudXBraS5jYS9zZXJ2aWNlL3Vzc28iLCJqdGkiOiJmOTZkNzgwYi02YmE3LTQ4ZTEtYjlmMC0wOTdkNmNiOWQxMTkiLCJwbGN5IjoiemdNTm5BSk9IRENwaUFXcGVJdVVicVBwb2k5bXdBQ1FuNmNaNWxOYkRtNy83Z0hDNVVGbFh2ZzF4QUR1b09GelRPNzFSNzN0aisxVjF1c25jRTZYc3RpNmMvU0t1akhwV3lSb09oVm5iZTgveVVOaGVkTFVxQndPV1NWQTNEY2MrRG9heFdtdVhJRVkiLCJwbGN5X2tleSI6ImtleS11c3NvLXBsY3ktMTEwOTE4LnBlbSIsInJvbGVzIjoiY2xpZW50LGFkbWluIiwic3ViIjoic3BpZmZlOi8vcGVyc29ubmVsLnVwa2kuY2EvZWlkLzEyNjQxMiIsInRva2VuIjoiY2U2YTBiZGMtYzRlNy00MGNjLTgzMTMtZDNiNDEzZTMwZmNkIiwidHlwZSI6Im9mZmxpbmUiLCJ1dWlkIjoiMTlhMTcxNDEtOWZkZi00ZGJmLThlMjgtZTRiYmVhZDMwNmVkIiwid29ua2EiOiJleUpqZENJNklsZFBUa3RCUXlJc0luWmhJam94TlRVd05UTTNOamd5TENKMllpSTZNVFUxTURZd09UYzBNaXdpWlNJNkltZGlZVzlBZFdKbGNpNWpiMjBpTENKaklqcGJJa1ZXUlZKWlQwNUZJaXdpWjJKaGIwQjFZbVZ5TG1OdmJTSmRMQ0prSWpvaWRYTnpieTEwWlhOMExYTnBkR1VpTENKeklqb2lLeTlGV0VVMFVsUm5OVU51Ulc0d09UTkhhbkpPV1VONFFXaDVNbk5wTUdaRFpFOTRVSHA0ZGpoS1EyZzVjRUZQWmxaRFVIcEthR1V5TkdSWFVsQnhaRWxVYmtoYU9FeHJVMVpSYUZCWmJEVmlVREZCWTFFOVBTSjkifQ.<signature>
```
If you don't have uSSH installed, your can authenticate through Onelogin and Duo in the command line console.

```
$ ./usso -login usso-test-site
```

If everything goes well you will see:

```
+-+-+-+-+ +-+-+-+ +-+-+-+-+
|u|s|s|o| |c|l|i| |t|o|o|l|
+-+-+-+-+ +-+-+-+ +-+-+-+-+

Enter your One-Login password: *******
Duo two-factor login for gbao@uber.com

Select one of the following options:

 1. Push notification via Duo app
 2. Passcode via SMS message

Choose option (1-2): 1

eyJhbGciOiJFUzI1NiIsImVudiI6ImRldiIsImtpZCI6InB1Yi11c3NvLXN2Yy0wOTE3MTgucGVtIiwidHlwIjoiSldUIiwidmVyIjoiMS4wIn0.eyJjbGllbnRfaWQiOiJ0ZXN0X2Zvby51YmVyaW50ZXJuYWwuY29tIiwiZW1haWwiOiJnYmFvQHViZXIuY29tIiwiZXhwIjoxNTQ0MDU1MjU0LCJpYXQiOjE1NDM5ODI5NTQsImlzcyI6InNwaWZmZTovL2NvbXB1dGUuaG9zdHMudXBraS5jYS9zZXJ2aWNlL3Vzc28iLCJqdGkiOiJmMDIxMDQ1Ny0xMzYzLTRiNTgtYjgzZC1mZTQxYWNiODI3OTEiLCJwbGN5IjoiRXp5NHlYKzJDTFJiV2RleGMxTXRoQ3BrYkNQS3BCWUIwREcyMlNRTXpxbmd5REVSVnIxZkxYZ3prZU9mdCs2MDgwRzFYWk1NaUk0clJaU25adG9vUGQ0VWszWjF1NExZdktCYkZBVT0iLCJwbGN5X2tleSI6ImtleS11c3NvLXBsY3ktMTEwOTE4LnBlbSIsInJvbGVzIjoiY2xpZW50Iiwic3ViIjoic3BpZmZlOi8vcGVyc29ubmVsLnVwa2kuY2EvdXNlci9nYmFvQHViZXIuY29tIiwidG9rZW4iOiJjZTZhMGJkYy1jNGU3LTQwY2MtODMxMy1kM2I0MTNlMzBmY2QiLCJ0eXBlIjoib2ZmbGluZSIsInV1aWQiOiIxOWExNzE0MS05ZmRmLTRkYmYtOGUyOC1lNGJiZWFkMzA2ZWQiLCJ3b25rYSI6Im1vY2tfd29ua2FfY2xhaW1fZ2Jhb0B1YmVyLmNvbV90ZXN0X2Zvb18zMzZfaG91cnMifQ.<signature>
```

To use the offline token, you can copy and paste the offline token as a bearer token into your curl command as shown below:

E.g.
```
curl -X GET https://usso-test-site.uberinternal.com -H 'Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiI...'
```

### How to import uSSO CLI tool as a Go package

If you don't want to run a seperate process, you can import uSSO CLI tool into your Golang based CLI tool by:

```
import "code.uber.internal/engsec/usso-cli.git/ussotoken"

// this is sample code shows how to use ussocli package to get offline token via ussh certs
func usshUSSO(domain string) int {
	ussh, err := ussotoken.NewUSSH()
	if err != nil {
		fmt.Println("Error: " + err.Error())
		return
	}
	offineTokenRes, err := ussh.GetOfflineToken(domain)
	if err != nil {
		fmt.Println("Error: " + err.Error())
		return
	}
	fmt.Println(offineTokenRes.Token)
	return
}

// this is sample code shows how to use ussocli package to get offline token via MFA
func loginUSSO(domain string) int {
	offineTokenRes, err := ussotoken.NewLogin(domain)
	if err != nil {
		fmt.Println("Error: " + err.Error())
		return
	}
	fmt.Println(offineTokenRes.Token)
	return
}
```

For more information
```sh
$ ./usso -help
```
Contact `eng-usso-group@uber.com` if you have any quesitons
