Testing Against Staging
=======================
The staging wonkamaster instance(s) can be contacting using the following environment variables:
```
WONKA_MASTER_ECC_PUBKEY=0222c15fa34947bbbd8bdcf30d1bb2ab340dbdca0fd1aa80e23df645480f6bae3
WONKA_MASTER_URL=http://127.0.0.1:16749
```

Local Development
=================

you only need to do steps 1-4 the first time.

1. install docker and docker-compose

 $ brew install docker docker-compose

2. start docker

3. edit pulloconfig in config/test.yaml to have the group memberships you want.
   use the existing entries for an example.

   you'll need AD:engineering to enroll new services. you'll need AD:wonka-admin to
   delete services.

4. create a new private key for wonkamaster. this is referenced in test.yaml in the
   'wonkamasterkey' var.

  $ cd wonkamaster # if you're in wonka-go.git
  $ openssl genrsa -out private.pem 4096
  $ openssl rsa -in private.pem -outform PEM -pubout -out public.pem

5. create a fake ssh known hosts key

  $ ssh-keygen -f ssh_known_hosts -N ""
  $ sed -i "" 's|^|@cert-authority \* |' ssh_known_hosts.pub

6. `cd ../`

7. Optional: Load development configuration using

   export UBER_ENVIRONMENT=development

8. `make db-install` (you'll need to have `cqlsh` installed by running `brew install cassandra`
   to get it.)

9. `make run` (you want to be in the base directory of the repo for this)

   This may fail the first time, if it does, re-run the command.


Now you have a wonkamaster instance running locally. To use this from wonkacli, you'll need to override
some wonkacli settings so claims signed by this wonkamster validate. to do so, you need to run wonkacli
with the following environment variables set.
```
WONKA_MASTER_HOST - 'localhost'
WONKA_MASTER_PORT - '16746' - from wonkamaster/config/test.yaml
WONKA_MASTER_ECC_PUB - the compressed wonkamaster ecc key
```

You can get the compressed wonkamaster ecc key from this log line when you `make run`:
```
{"wonka":{"compressed":"0213423267c062735a8b21ad96629eb4fa6b145dec9e17ff8cfdbecdf1e8549981","eccX":"13423267c062735a8b21ad96629eb4fa6b145dec9e17ff8cfdbecdf1e8549981","eccY":"79b56f4d4ad1baed977d8c926066e3a3b0a71e8ded76b7b613a3099cc76beca0","entity":"wonkamaster","level":"debug"},"msg":"wonka.go:269 server ecc key","time":"2017-07-19T16:26:25-07:00"}
```
So running wonkacli against our local wonkamaster looks like:
```
$ cd wonkacli
$ openssl genrsa -out wonka_private 4096
$ openssl rsa -in wonka_private -outform PEM -pubout -out wonka_public

$ WONKA_MASTER_HOST=localhost WONKA_MASTER_PORT=16746 \
  WONKA_MASTER_ECC_PUB=0213423267c062735a8b21ad96629eb4fa6b145dec9e17ff8cfdbecdf1e8549981 \
  ./wonkacli enroll --name wonkaSample:test -g EVERYONE --privkey wonka_private

$ WONKA_MASTER_HOST=localhost WONKA_MASTER_PORT=16746 \
  WONKA_MASTER_ECC_PUB=0213423267c062735a8b21ad96629eb4fa6b145dec9e17ff8cfdbecdf1e8549981 \
  ./wonkacli lookup --name wonkaSample:test
```
This will enroll a new service, wonkaSample:test with our local wonkamaster and then do a lookup of the entity.
