package openstack

import (
	"context"
	"net"
	"sync"
	"time"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/pkg/dnsutil"
	"github.com/coredns/coredns/plugin/pkg/fall"
	"github.com/coredns/coredns/request"
	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/openstack"
	"github.com/gophercloud/gophercloud/openstack/compute/v2/servers"
	"github.com/gophercloud/gophercloud/pagination"
	"github.com/miekg/dns"
)

type Map struct {
	name4 map[string][]net.IP
	addr  map[string][]string
}

func newMap() *Map {
	return &Map{
		name4: make(map[string][]net.IP),
		addr:  make(map[string][]string),
	}
}

type OpenStack struct {
	sync.RWMutex

	Next plugin.Handler
	Fall fall.F

	origins []string
	hmap    *Map
	ttl     uint32
	reload  time.Duration

	authOptions gophercloud.AuthOptions
	region      string
}

func (os *OpenStack) Name() string {
	return "openstack"
}

func (os *OpenStack) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	state := request.Request{W: w, Req: r}
	qname := state.Name()
	var answers []dns.RR

	zone := plugin.Zones(os.origins).Matches(qname)
	if zone == "" {
		if state.QType() != dns.TypePTR {
			return plugin.NextOrFailure(os.Name(), os.Next, ctx, w, r)
		}
	}

	switch state.QType() {
	case dns.TypePTR:
		names := os.LookupAddr(dnsutil.ExtractAddressFromReverse(qname))
		if len(names) == 0 {
			return plugin.NextOrFailure(os.Name(), os.Next, ctx, w, r)
		}
		answers = ptr(qname, os.ttl, names)
	case dns.TypeA:
		ips := os.lookupHostV4(qname)
		answers = a(qname, os.ttl, ips)
	}

	if len(answers) == 0 {
		if os.Fall.Through(qname) {
			return plugin.NextOrFailure(os.Name(), os.Next, ctx, w, r)
		}

		return dns.RcodeServerFailure, nil
	}

	m := new(dns.Msg)
	m.SetReply(r)
	m.Authoritative = true
	m.Answer = answers

	if err := w.WriteMsg(m); err != nil {
		log.Error(err)
	}

	return dns.RcodeSuccess, nil
}

func (os *OpenStack) extractAddrs(serverAddresses map[string]interface{}) []net.IP {
	addrs := make([]net.IP, 0)
	for _, addrList := range serverAddresses {
		t := addrList.([]interface{})
		for _, j := range t {
			k := j.(map[string]interface{})
			if k["OS-EXT-IPS:type"] != "floating" && k["version"].(float64) == 4 {
				addrs = append(addrs, net.ParseIP(k["addr"].(string)))
			}
		}
	}

	return addrs
}

func (os *OpenStack) updateMap() {
	log.Debugf("Updating server list")
	hmap := newMap()

	client, err := openstack.AuthenticatedClient(os.authOptions)
	if err != nil {
		log.Errorf("Authentication failed: %s", err)
		return
	}

	compute, err := openstack.NewComputeV2(client, gophercloud.EndpointOpts{
		Region: os.region,
	})
	if err != nil {
		log.Errorf("Failed to fetch servers: %s", err)
		return
	}

	pager := servers.List(compute, servers.ListOpts{})
	err = pager.EachPage(func(page pagination.Page) (bool, error) {
		serverList, err := servers.ExtractServers(page)
		if err != nil {
			return false, err
		}
		for _, s := range serverList {
			addrs := os.extractAddrs(s.Addresses)
			for _, zone := range os.origins {
				name := dnsutil.Join(s.Name, zone)

				for _, addr := range addrs {
					hmap.name4[name] = append(hmap.name4[name], addr)
					hmap.addr[addr.String()] = append(hmap.addr[addr.String()], name)
				}
			}
		}

		return true, nil
	})

	if err != nil {
		log.Errorf("Failed to list servers: %s", err)
		return
	}

	os.Lock()
	os.hmap = hmap
	os.Unlock()
}

func (os *OpenStack) lookupHostV4(host string) []net.IP {
	os.RLock()
	defer os.RUnlock()

	ips, ok := os.hmap.name4[host]
	if !ok {
		return nil
	}

	ipsCp := make([]net.IP, len(ips))
	copy(ipsCp, ips)
	return ipsCp
}

func (os *OpenStack) LookupAddr(addr string) []string {
	addr = net.ParseIP(addr).String()
	if addr == "" {
		return nil
	}

	os.RLock()
	defer os.RUnlock()
	hosts := os.hmap.addr[addr]

	if len(hosts) == 0 {
		return nil
	}

	hostsCp := make([]string, len(hosts))
	copy(hostsCp, hosts)
	return hostsCp
}

func a(zone string, ttl uint32, ips []net.IP) []dns.RR {
	answers := make([]dns.RR, len(ips))
	for i, ip := range ips {
		r := new(dns.A)
		r.Hdr = dns.RR_Header{Name: zone, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: ttl}
		r.A = ip
		answers[i] = r
	}
	return answers
}

func ptr(zone string, ttl uint32, names []string) []dns.RR {
	answers := make([]dns.RR, len(names))
	for i, n := range names {
		r := new(dns.PTR)
		r.Hdr = dns.RR_Header{Name: zone, Rrtype: dns.TypePTR, Class: dns.ClassINET, Ttl: ttl}
		r.Ptr = dns.Fqdn(n)
		answers[i] = r
	}
	return answers
}
