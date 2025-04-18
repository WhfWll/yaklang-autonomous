package match

import (
	"encoding/binary"
	"fmt"
	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/yaklang/yaklang/common/suricata/data/modifier"
)

func tcpParser(c *matchContext) error {
	if !c.Must(c.Rule.ContentRuleConfig != nil) {
		return nil
	}

	// buffer provider
	provider := newTCPProvider(c.PK)
	if !c.Must(provider != nil) {
		return nil
	}

	// register buffer provider
	c.SetBufferProvider(provider)

	return nil
}

func tcpCfgMatch(c *matchContext) error {
	if c.Rule.ContentRuleConfig.TcpConfig == nil {
		return nil
	}
	tcpConfig := c.Rule.ContentRuleConfig.TcpConfig

	tcp, ok := c.PK.Layer(layers.LayerTypeTCP).(*layers.TCP)
	if !ok {
		return fmt.Errorf("tcp layer not found")
	}

	if tcpConfig.Seq != nil && !c.Must(*tcpConfig.Seq == int(tcp.Seq)) {
		return nil
	}

	if tcpConfig.Ack != nil && !c.Must(*tcpConfig.Ack == int(tcp.Ack)) {
		return nil
	}

	if tcpConfig.Window != nil &&
		!c.Must(negIf(tcpConfig.NegativeWindow, *tcpConfig.Window == int(tcp.Window))) {
		return nil
	}

	if tcpConfig.TCPMss != nil {
		var mss int
		var set bool
		for _, opt := range tcp.Options {
			if opt.OptionType == layers.TCPOptionKindMSS {
				mss = int(binary.BigEndian.Uint16(opt.OptionData))
				set = true
				break
			}
		}
		// if option mss not found, skip
		if !c.Must(set) {
			return nil
		}

		if !c.Must(tcpConfig.TCPMss.Match(mss)) {
			return nil
		}
	}

	return nil
}

func newTCPProvider(pk gopacket.Packet) func(modifier modifier.Modifier) []byte {
	tcp, ok := pk.Layer(layers.LayerTypeTCP).(*layers.TCP)
	if !ok {
		return nil
	}
	return func(mdf modifier.Modifier) []byte {
		switch mdf {
		case modifier.TCPHDR:
			return tcp.Contents
		case modifier.Default:
			return tcp.Payload
		}
		return nil
	}
}
