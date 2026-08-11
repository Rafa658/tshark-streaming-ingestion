package shape

import (
	"encoding/json"
	"net/netip"
	"testing"
)

func ekLine(t *testing.T, overrides ...func(map[string]any)) []byte {
	t.Helper()
	doc := map[string]any{
		"timestamp": "1770000000789",
		"layers": map[string]any{
			"frame": map[string]any{
				"frame_frame_protocols": "eth:ethertype:ip:tcp",
				"frame_frame_len":        "1234",
			},
			"ip":  map[string]any{"ip_ip_src": "192.168.1.100", "ip_ip_dst": "10.0.0.200"},
			"tcp": map[string]any{"tcp_tcp_srcport": "443", "tcp_tcp_dstport": "54321"},
		},
	}
	for _, o := range overrides {
		o(doc)
	}
	b, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func withLayers(layers map[string]any) func(map[string]any) {
	return func(doc map[string]any) { doc["layers"] = layers }
}

func TestShapeBasic(t *testing.T) {
	row, err := Shape(ekLine(t))
	if err != nil {
		t.Fatal(err)
	}
	if row.TS.Year() != 2026 {
		t.Errorf("ts year = %d, want 2026", row.TS.Year())
	}
	if row.TS.Location().String() != "UTC" {
		t.Errorf("ts location = %s, want UTC", row.TS.Location())
	}
	srcIP, _ := netip.ParseAddr("192.168.1.100")
	if row.SrcIP != srcIP {
		t.Errorf("src_ip = %s, want 192.168.1.100", row.SrcIP)
	}
	dstIP, _ := netip.ParseAddr("10.0.0.200")
	if row.DstIP != dstIP {
		t.Errorf("dst_ip = %s, want 10.0.0.200", row.DstIP)
	}
	if row.SrcPort == nil || *row.SrcPort != 443 {
		t.Errorf("src_port = %v, want 443", row.SrcPort)
	}
	if row.DstPort == nil || *row.DstPort != 54321 {
		t.Errorf("dst_port = %v, want 54321", row.DstPort)
	}
	if row.Proto != "eth:ethertype:ip:tcp" {
		t.Errorf("proto = %s", row.Proto)
	}
	if row.Length != 1234 {
		t.Errorf("length = %d, want 1234", row.Length)
	}
	var payload map[string]any
	if err := json.Unmarshal(row.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if _, ok := payload["layers"]; !ok {
		t.Error("payload missing 'layers' key")
	}
}

func TestShapeTimestampUTC(t *testing.T) {
	row, err := Shape(ekLine(t, func(d map[string]any) { d["timestamp"] = "0" }))
	if err != nil {
		t.Fatal(err)
	}
	if row.TS.Unix() != 0 {
		t.Errorf("ts unix = %d, want 0", row.TS.Unix())
	}
	if row.TS.Location().String() != "UTC" {
		t.Errorf("ts location = %s, want UTC", row.TS.Location())
	}
}

func TestShapeUDPPorts(t *testing.T) {
	layers := map[string]any{
		"frame": map[string]any{"frame_frame_protocols": "eth:ip:udp", "frame_frame_len": "60"},
		"ip":    map[string]any{"ip_ip_src": "192.168.1.1", "ip_ip_dst": "8.8.8.8"},
		"udp":   map[string]any{"udp_udp_srcport": "53", "udp_udp_dstport": "5353"},
	}
	row, err := Shape(ekLine(t, withLayers(layers)))
	if err != nil {
		t.Fatal(err)
	}
	if row.SrcPort == nil || *row.SrcPort != 53 {
		t.Errorf("src_port = %v, want 53", row.SrcPort)
	}
	if row.DstPort == nil || *row.DstPort != 5353 {
		t.Errorf("dst_port = %v, want 5353", row.DstPort)
	}
}

func TestShapeIPv6(t *testing.T) {
	layers := map[string]any{
		"frame": map[string]any{"frame_frame_protocols": "eth:ipv6:tcp", "frame_frame_len": "80"},
		"ipv6":  map[string]any{"ipv6_ipv6_src": "2001:db8::1", "ipv6_ipv6_dst": "2001:db8::2"},
		"tcp":   map[string]any{"tcp_tcp_srcport": "80", "tcp_tcp_dstport": "8080"},
	}
	row, err := Shape(ekLine(t, withLayers(layers)))
	if err != nil {
		t.Fatal(err)
	}
	srcIP, _ := netip.ParseAddr("2001:db8::1")
	if row.SrcIP != srcIP {
		t.Errorf("src_ip = %s, want 2001:db8::1", row.SrcIP)
	}
	dstIP, _ := netip.ParseAddr("2001:db8::2")
	if row.DstIP != dstIP {
		t.Errorf("dst_ip = %s, want 2001:db8::2", row.DstIP)
	}
}

func TestShapeNoTransportLayerHasNilPorts(t *testing.T) {
	layers := map[string]any{
		"frame": map[string]any{"frame_frame_protocols": "eth:ip:icmp", "frame_frame_len": "98"},
		"ip":    map[string]any{"ip_ip_src": "192.168.1.1", "ip_ip_dst": "192.168.1.2"},
	}
	row, err := Shape(ekLine(t, withLayers(layers)))
	if err != nil {
		t.Fatal(err)
	}
	if row.SrcPort != nil {
		t.Errorf("src_port = %v, want nil", row.SrcPort)
	}
	if row.DstPort != nil {
		t.Errorf("dst_port = %v, want nil", row.DstPort)
	}
}

func TestShapeMissingIPRaisesError(t *testing.T) {
	layers := map[string]any{
		"frame": map[string]any{"frame_frame_protocols": "eth:ip", "frame_frame_len": "100"},
	}
	_, err := Shape(ekLine(t, withLayers(layers)))
	if err == nil {
		t.Fatal("expected error for missing IP addresses")
	}
}

func TestShapeInvalidIPRaisesError(t *testing.T) {
	layers := map[string]any{
		"frame": map[string]any{"frame_frame_protocols": "eth:ip", "frame_frame_len": "100"},
		"ip":    map[string]any{"ip_ip_src": "invalid", "ip_ip_dst": "10.0.0.1"},
	}
	_, err := Shape(ekLine(t, withLayers(layers)))
	if err == nil {
		t.Fatal("expected error for invalid IP")
	}
}

func TestShapeMissingTimestampRaisesError(t *testing.T) {
	line := ekLine(t, func(d map[string]any) { delete(d, "timestamp") })
	_, err := Shape(line)
	if err == nil {
		t.Fatal("expected error for missing timestamp")
	}
}

func TestShapeFlexStringHandlesNumber(t *testing.T) {
	doc := map[string]any{
		"timestamp": 1770000000789,
		"layers": map[string]any{
			"frame": map[string]any{"frame_frame_protocols": "eth:ip:tcp", "frame_frame_len": 1234},
			"ip":    map[string]any{"ip_ip_src": "192.168.1.1", "ip_ip_dst": "10.0.0.1"},
			"tcp":   map[string]any{"tcp_tcp_srcport": 443, "tcp_tcp_dstport": 80},
		},
	}
	b, _ := json.Marshal(doc)
	row, err := Shape(b)
	if err != nil {
		t.Fatalf("flexString should handle numbers: %v", err)
	}
	if row.Length != 1234 {
		t.Errorf("length = %d, want 1234", row.Length)
	}
	if row.SrcPort == nil || *row.SrcPort != 443 {
		t.Errorf("src_port = %v, want 443", row.SrcPort)
	}
}
