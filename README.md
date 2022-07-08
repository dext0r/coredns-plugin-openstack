# openstack

## Name

*openstack* - enables serving zone data from OpenStack Compute API.

## Description

This plugin allows to resolve server names into the corresponding IP addresses using OpenStack Compute API.

## Syntax

```
openstack [ZONES...] {
    auth_url AUTHENTICATION_URL
    username USERNAME
    passwork PASSWORD
    tenant_id TENANT_ID
    domain_name DOMAIN_NAME
    region REGION_NAME
    ttl SECONDS
    reload DURATION
    fallthrough [ZONES...]
}
```

* **ZONES** zones it should be authoritative for. If empty, the zones from the configuration block
   are used.
* `auth_url` specifies the Keystone authentication URL.
* `username` specifies the name of a user who can list tenants and list all servers.
* `password` specifies the password of the user.
* `domain_name` specifies the Keystone domain which the user belongs to. The default is `default`.
* `tenant_id` specifies the ID of the Tenant (Identity v2) or Project (Identity v3) to login with.
* `region` specifies the OpenStack region for your servers. The default is `RegionOne`.
* `ttl` change the DNS TTL of the records generated (forward and reverse). The default is 3600 seconds (1 hour).
* `reload` change the period between reload data from OpenStack. A time of zero seconds disables the
  feature. Examples of valid durations: "300ms", "1.5h" or "2h45m". See Go's
  [time](https://godoc.org/time) package. The default is 30 seconds.
* `fallthrough` If zone matches and no record can be generated, pass request to the next plugin.
  If **[ZONES...]** is omitted, then fallthrough happens for all zones for which the plugin
  is authoritative. If specific zones are listed (for example `in-addr.arpa` and `ip6.arpa`), then only
  queries for those zones will be subject to fallthrough.

## Examples

Mail.ru Cloud

```
. {
    openstack example.net {
        auth_url https://infra.mail.ru:35357/v3
        username mail@example.net
        passwork PASSWORD
        domain_name users
        tenant_id xxx  # Project settings -> API Keys -> Project ID
        region RegionOne
        ttl 60
        reload 120
    }
    errors
    log
}
```
