flashlight [![Travis CI Status](https://travis-ci.org/getlantern/flashlight.svg?branch=master)](https://travis-ci.org/getlantern/flashlight)&nbsp;[![Coverage Status](https://coveralls.io/repos/getlantern/flashlight/badge.png)](https://coveralls.io/r/getlantern/flashlight)&nbsp;[![GoDoc](https://godoc.org/github.com/getlantern/flashlight?status.png)](http://godoc.org/github.com/getlantern/flashlight)
==========

Lightweight host-spoofing web proxy written in go.

flashlight runs in one of two modes:

client - meant to run locally to wherever the browser is running, forwards
requests to the server

server - handles requests from a flashlight client proxy and actually proxies
them to the final destination

Using CloudFlare (and other CDNS), flashlight has the ability to masquerade as
running on a different domain than it is.  The client simply specifies the
"masquerade" flag with a value like "thehackernews.com".  flashlight will then
use that masquerade host for the DNS lookup and will also specify it as the
ServerName for SNI (though this is not actually necessary on CloudFlare). The
Host header of the HTTP request will actually contain the correct host
(e.g. getiantem.org), which causes CloudFlare to route the request to the
correct host.

Flashlight uses [enproxy](https://github.com/getlantern/enproxy) to encapsulate
data from/to the client as http request/response pairs.  This allows it to
tunnel regular HTTP as well as HTTPS traffic over CloudFlare.  In fact, it can
tunnel any TCP traffic.

### Usage

```bash
Usage of flashlight:
  -addr (required): ip:port on which to listen for requests.  When running as a client proxy, we'll listen with http, when running as a server proxy we'll listen with https
  -configdir="": directory in which to store configuration (defaults to current directory)
  -cpuprofile="": write cpu profile to given file
  -dumpheaders=false: dump the headers of outgoing requests and responses to stdout
  -help=false: Get usage help
  -instanceid="": instanceId under which to report stats to statshub.  If not specified, no stats are reported.
  -masquerade="": masquerade host: if specified, flashlight will actually make a request to this host's IP but with a host header corresponding to the 'server' parameter
  -role (required): either 'client' or 'server'
  -rootca="": pin to this CA cert if specified (PEM format)
  -server (required): FQDN of flashlight server
  -serverport=443: the port on which to connect to the server
```

-rootca needs to be the complete PEM data, with header and trailer and all
newlines, for example:

```
flashlight -addr localhost:10080 -server localhost -serverport 10081 -rootca "-----BEGIN CERTIFICATE-----
MIIC/jCCAeigAwIBAgIEI6PHvjALBgkqhkiG9w0BAQswJjEQMA4GA1UEChMHTGFu
dGVybjESMBAGA1UEAxMJbG9jYWxob3N0MB4XDTE0MDUwMzE5NTQzMFoXDTI0MDYw
MzE5NTQzMFowJjEQMA4GA1UEChMHTGFudGVybjESMBAGA1UEAxMJbG9jYWxob3N0
MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEAzYeEJ/wJMeu0LA9/DLuw
n0j9HmAu/CK34e1jXsUuGkuheLLYWC32jVMsQdYaWuv8wFf2soXYH3WoEfOUkpTJ
N53WA4mmRd2nZidUxvUIiLdcQlJf+xar7vJih5MgsMYmVR+r7C1fLYlONuFpM6XV
5VuixGZyOcrLcOBbW1NimZLDzFYqAMy6l6U3eKvjK8KasnPURlAnVKRLquf4WA41
diQXWAzJCVgPz/f3Z4nL/SCADOkc2nGOroh63xbIra1eQdKfn8fOU1qeq/Bl1gPq
OdnSTGO19quSyf8XB6bDyl3TNeBCV5/FLIp8fjFzVdPAdZFjmMWTv3ccCEpmjsZe
xwIDAQABozgwNjAOBgNVHQ8BAf8EBAMCAKQwEwYDVR0lBAwwCgYIKwYBBQUHAwEw
DwYDVR0TAQH/BAUwAwEB/zALBgkqhkiG9w0BAQsDggEBAFLDvZBjdhLZuyHL3q6G
ZC93zaGkpdS8ux3gw4lldtr/SYW8aJ9Ck4+aGv7kouFylAAmxUXODUqh8vG1mc7D
uGHn5DHzHjlY1pSaedhcDcWIk1WB7ENoncWI9ZoutP3A4A+GTjwK35G7gBCP6bD+
qI6VIezWU0oFlFOgTdIKHNEbFpEgIUm1WUhrQ1zzRGVNVNxo4YZyqxe3pVKNwSmx
QggkGR2oOUVjfoyZ3pbUca4YnxiDgWRnbehgdK6Acq0kT9SCYAP0qTXCwZTeRJog
Na7vvprDERbUvc9c0rSUGHUrKqbf5AAmStI6fHGTNvdOMHZfoekwrE0CbyWcX/UH
gcA=
-----END CERTIFICATE-----"
```

**IMPORTANT** - when running a test locally, run the server first, then pass the
contents of servercert.pem to the client flashlight with the -rootca flag.  This
way the client will trust the local server, which is using a self-signed cert.

Example Client:

```bash 
./flashlight -addr localhost:10080 -role client
```

Example Server:

```bash
./flashlight -addr :443
```

Example Curl Test:

```bash
curl -x localhost:10080 http://www.google.com/humans.txt
Google is built by a large team of engineers, designers, researchers, robots, and others in many different sites across the globe. It is updated continuously, and built with more tools and technologies than we can shake a stick at. If you'd like to help us out, see google.com/careers.
```

On the client, you should see something like this for every request:

```bash
Handling request for: http://www.google.com/humans.txt
```

### Building

Flashlight requires [Go 1.3](http://golang.org/dl/).

It is convenient to build flashlight for multiple platforms using something like
[goxc](https://github.com/laher/goxc).

With goxc, the binaries used for Lantern can be built using the
./crosscompile.bash script. This script also sets the version of flashlight to
the most recent annotated tag in git. An annotated tag can be added like this:

`git tag -a v1.0.0 -m"Tagged 1.0.0"`

Note - ./crosscompile.bash omits debug symbols to keep the build smaller.

The binaries end up at
`$GOPATH/bin/flashlight-xc/snapshot/<platform>/flashlight`.

Note that these binaries should also be signed for use in production, at least
on OSX and Windows. On OSX the command to do this should resemble the following
(assuming you have an associated code signing certificate):

```
codesign -s "Developer ID Application: Brave New Software Project, Inc" -f install/osx/pt/flashlight/flashlight
```

### Masquerade Host Management

Masquerade host configuration is managed using utilities in the certs/ subfolder.

#### Setup

You need python 2.7 and the following packages:

```bash
pip install pyyaml
pip install jinja2
pip install --upgrade pyopenssl
```
Notes:
- If you're not using virtual environments, you may need to sudo all of these commands.
- This requires a fairly recent version of OpenSSL (more recent than what is installed with OS X).

In addition, you need the s3cmd tool installed and set up.  To install on
Ubuntu:

```bash
sudo apt-get install s3cmd
```

On OS X:
```bash
brew install s3cmd
```

And then run `s3cmd --configure` and follow the on-screen instructions.  You
can get AWS credentials that are good for uploading to S3 in
[too-many-secrets/lantern_aws/aws_credential](https://github.com/getlantern/too-many-secrets/blob/master/lantern_aws/aws_credential).

#### Adding new masquerade hosts

Compile the list of domains in a file, separated with whitespace (e.g., one
per line), cd to the certs/ subfolder, and run `./addmasquerades.py <your file>`.

#### Removing masquerade hosts

Remove the corresponding cert file from the certs/ subfolder, cd to that
directory and run `./addmasquerades.py nodomains.txt`.

#### Refreshing the root CA certs for hosts

Run `./refreshcerts.py [<your file>]`, where the file, if provided, should
have the same format as for `addmasquerades.py`.  If no domains file is
provided, the root CA certs for all domains will be refreshed.
