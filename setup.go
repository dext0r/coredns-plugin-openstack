package openstack

import (
	"strconv"
	"time"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"
	clog "github.com/coredns/coredns/plugin/pkg/log"
	"github.com/gophercloud/gophercloud"
)

var log = clog.NewWithPlugin("openstack")

func init() { plugin.Register("openstack", setup) }

func periodicUpdateMap(os *OpenStack) chan bool {
	updateChan := make(chan bool)

	if os.reload == 0 {
		return updateChan
	}

	go func() {
		ticker := time.NewTicker(os.reload)
		for {
			select {
			case <-updateChan:
				return
			case <-ticker.C:
				os.updateMap()
			}
		}
	}()

	return updateChan
}

func setup(c *caddy.Controller) error {
	os, err := openstackParse(c)
	if err != nil {
		return plugin.Error("openstack", err)
	}

	updateChan := periodicUpdateMap(os)

	c.OnStartup(func() error {
		os.updateMap()
		return nil
	})

	c.OnShutdown(func() error {
		close(updateChan)
		return nil
	})

	dnsserver.GetConfig(c).AddPlugin(func(next plugin.Handler) plugin.Handler {
		os.Next = next
		return os
	})

	return nil
}

func openstackParse(c *caddy.Controller) (*OpenStack, error) {
	os := OpenStack{
		hmap:   newMap(),
		region: "RegionOne",
		authOptions: gophercloud.AuthOptions{
			Username:   "coredns",
			DomainName: "default",
		},
		ttl:    3600,
		reload: 30 * time.Second,
	}

	i := 0
	for c.Next() {
		if i > 0 {
			return &os, plugin.ErrOnce
		}
		i++

		os.origins = plugin.OriginsFromArgsOrServerBlock(c.RemainingArgs(), c.ServerBlockKeys)

		for c.NextBlock() {
			switch c.Val() {
			case "auth_url":
				args := c.RemainingArgs()
				if len(args) != 1 {
					return nil, c.ArgErr()
				}
				os.authOptions.IdentityEndpoint = args[0]
			case "username":
				args := c.RemainingArgs()
				if len(args) != 1 {
					return nil, c.ArgErr()
				}
				os.authOptions.Username = args[0]
			case "password":
				args := c.RemainingArgs()
				if len(args) != 1 {
					return nil, c.ArgErr()
				}
				os.authOptions.Password = args[0]
			case "domain_name":
				args := c.RemainingArgs()
				if len(args) != 1 {
					return nil, c.ArgErr()
				}
				os.authOptions.DomainName = args[0]
			case "tenant_id":
				args := c.RemainingArgs()
				if len(args) != 1 {
					return nil, c.ArgErr()
				}
				os.authOptions.TenantID = args[0]
			case "region":
				args := c.RemainingArgs()
				if len(args) != 1 {
					return nil, c.ArgErr()
				}
				os.region = args[0]
			case "ttl":
				args := c.RemainingArgs()
				if len(args) != 1 {
					return nil, c.ArgErr()
				}
				ttl, err := strconv.Atoi(args[0])
				if err != nil {
					return nil, c.Errf("ttl needs a number of second")
				}
				if ttl <= 0 || ttl > 65535 {
					return nil, c.Errf("ttl provided is invalid")
				}
				os.ttl = uint32(ttl)
			case "reload":
				args := c.RemainingArgs()
				if len(args) != 1 {
					return nil, c.ArgErr()
				}
				reload, err := time.ParseDuration(args[0])
				if err != nil {
					return nil, c.Errf("invalid duration for reload '%s'", args[0])
				}
				if reload < 0 {
					return nil, c.Errf("invalid negative duration for reload '%s'", args[0])
				}
				os.reload = reload
			case "fallthrough":
				os.Fall.SetZonesFromArgs(c.RemainingArgs())
			default:
				return nil, c.Errf("unknown property '%s'", c.Val())
			}
		}
	}

	return &os, nil
}
