# dnsq

dig but worse.

## Usage

```
dnsq [@resolver] <domain> [A|AAAA|NS|CNAME]
```

```
$ dnsq google.com
google.com 172.253.134.138 A IN 283
google.com 172.253.134.113 A IN 283
...

$ dnsq @8.8.8.8 google.com AAAA
google.com 2404:6800:4007:804::200e AAAA IN 300

$ dnsq @1.1.1.1 google.com NS
google.com ns1.google.com NS IN 337848
google.com ns3.google.com NS IN 337848
...
```

## Build

```
go build -o dnsq .
```
