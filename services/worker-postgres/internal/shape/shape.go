package shape

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/netip"
	"strconv"
	"time"
)

// Row is the shaped representation of a single ek line, ready for INSERT.
type Row struct {
	TS      time.Time
	SrcIP   netip.Addr
	DstIP   netip.Addr
	SrcPort *int
	DstPort *int
	Proto   string
	Length  int
	Payload json.RawMessage
}

// flexString accepts both JSON strings and numbers, normalising to a string.
// tshark ek emits field values as strings, but the type is not guaranteed
// across versions, so this matches Python's int(str_or_num) coercion.
type flexString string

func (f *flexString) UnmarshalJSON(data []byte) error {
	data = bytes.TrimSpace(data)
	if len(data) == 0 || string(data) == "null" {
		*f = ""
		return nil
	}
	if data[0] == '"' {
		var s string
		if err := json.Unmarshal(data, &s); err != nil {
			return err
		}
		*f = flexString(s)
		return nil
	}
	*f = flexString(data)
	return nil
}

type ekFrame struct {
	FrameProtocols flexString `json:"frame_frame_protocols"`
	FrameLen       flexString `json:"frame_frame_len"`
}

type ekIP struct {
	IPSrc flexString `json:"ip_ip_src"`
	IPDst flexString `json:"ip_ip_dst"`
}

type ekIPv6 struct {
	IPv6Src flexString `json:"ipv6_ipv6_src"`
	IPv6Dst flexString `json:"ipv6_ipv6_dst"`
}

type ekTCP struct {
	SrcPort flexString `json:"tcp_tcp_srcport"`
	DstPort flexString `json:"tcp_tcp_dstport"`
}

type ekUDP struct {
	SrcPort flexString `json:"udp_udp_srcport"`
	DstPort flexString `json:"udp_udp_dstport"`
}

type ekDocument struct {
	Timestamp flexString `json:"timestamp"`
	Layers    struct {
		Frame ekFrame `json:"frame"`
		IP    *ekIP   `json:"ip"`
		IPv6  *ekIPv6 `json:"ipv6"`
		TCP   *ekTCP  `json:"tcp"`
		UDP   *ekUDP  `json:"udp"`
	} `json:"layers"`
}

// Shape converts a raw ek line into a Row. The payload is carried through as
// json.RawMessage so the original bytes reach Postgres without re-marshalling,
// preserving byte-parity with the Python worker's Json(payload) column.
func Shape(line []byte) (Row, error) {
	var ek ekDocument
	if err := json.Unmarshal(line, &ek); err != nil {
		return Row{}, fmt.Errorf("unmarshal ek: %w", err)
	}

	tsMs, err := strconv.ParseInt(string(ek.Timestamp), 10, 64)
	if err != nil {
		return Row{}, fmt.Errorf("parse timestamp %q: %w", ek.Timestamp, err)
	}
	ts := time.UnixMilli(tsMs).UTC()

	var srcStr, dstStr string
	if ek.Layers.IP != nil {
		srcStr = string(ek.Layers.IP.IPSrc)
		dstStr = string(ek.Layers.IP.IPDst)
	} else if ek.Layers.IPv6 != nil {
		srcStr = string(ek.Layers.IPv6.IPv6Src)
		dstStr = string(ek.Layers.IPv6.IPv6Dst)
	}
	if srcStr == "" || dstStr == "" {
		return Row{}, fmt.Errorf("missing IP addresses: src=%s, dst=%s", srcStr, dstStr)
	}

	srcIP, err := netip.ParseAddr(srcStr)
	if err != nil {
		return Row{}, fmt.Errorf("invalid src IP %q: %w", srcStr, err)
	}
	dstIP, err := netip.ParseAddr(dstStr)
	if err != nil {
		return Row{}, fmt.Errorf("invalid dst IP %q: %w", dstStr, err)
	}

	var srcPort, dstPort *int
	if ek.Layers.TCP != nil {
		srcPort, err = portPtr(ek.Layers.TCP.SrcPort)
		if err != nil {
			return Row{}, err
		}
		dstPort, err = portPtr(ek.Layers.TCP.DstPort)
		if err != nil {
			return Row{}, err
		}
	} else if ek.Layers.UDP != nil {
		srcPort, err = portPtr(ek.Layers.UDP.SrcPort)
		if err != nil {
			return Row{}, err
		}
		dstPort, err = portPtr(ek.Layers.UDP.DstPort)
		if err != nil {
			return Row{}, err
		}
	}

	length, err := parseIntField(ek.Layers.Frame.FrameLen, "frame_frame_len")
	if err != nil {
		return Row{}, err
	}

	return Row{
		TS:      ts,
		SrcIP:   srcIP,
		DstIP:   dstIP,
		SrcPort: srcPort,
		DstPort: dstPort,
		Proto:   string(ek.Layers.Frame.FrameProtocols),
		Length:  length,
		Payload: json.RawMessage(line),
	}, nil
}

func portPtr(s flexString) (*int, error) {
	if string(s) == "" {
		return nil, nil
	}
	n, err := strconv.Atoi(string(s))
	if err != nil {
		return nil, fmt.Errorf("invalid port %q: %w", s, err)
	}
	return &n, nil
}

func parseIntField(s flexString, name string) (int, error) {
	if string(s) == "" {
		return 0, nil
	}
	n, err := strconv.Atoi(string(s))
	if err != nil {
		return 0, fmt.Errorf("invalid %s %q: %w", name, s, err)
	}
	return n, nil
}
