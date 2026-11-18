package controller_test

import (
	"bytes"
	"encoding/base64"
	"testing"

	"github.com/xtls/xray-core/infra/conf"

	"github.com/wyx2685/XrayR/api"
	"github.com/wyx2685/XrayR/common/mylego"
	. "github.com/wyx2685/XrayR/service/controller"
)

func TestBuildV2ray(t *testing.T) {
	nodeInfo := &api.NodeInfo{
		NodeType:          "V2ray",
		NodeID:            1,
		Port:              1145,
		SpeedLimit:        0,
		AlterID:           2,
		TransportProtocol: "ws",
		Host:              "test.test.tk",
		Path:              "v2ray",
		EnableTLS:         false,
	}
	certConfig := &mylego.CertConfig{
		CertMode:   "http",
		CertDomain: "test.test.tk",
		Provider:   "alidns",
		Email:      "test@gmail.com",
	}
	config := &Config{
		CertConfig: certConfig,
	}
	_, err := InboundBuilder(config, nodeInfo, "test_tag", nil)
	if err != nil {
		t.Error(err)
	}
}

func TestBuildTrojan(t *testing.T) {
	nodeInfo := &api.NodeInfo{
		NodeType:          "Trojan",
		NodeID:            1,
		Port:              1145,
		SpeedLimit:        0,
		AlterID:           2,
		TransportProtocol: "tcp",
		Host:              "trojan.test.tk",
		Path:              "v2ray",
		EnableTLS:         false,
	}
	DNSEnv := make(map[string]string)
	DNSEnv["ALICLOUD_ACCESS_KEY"] = "aaa"
	DNSEnv["ALICLOUD_SECRET_KEY"] = "bbb"
	certConfig := &mylego.CertConfig{
		CertMode:   "dns",
		CertDomain: "trojan.test.tk",
		Provider:   "alidns",
		Email:      "test@gmail.com",
		DNSEnv:     DNSEnv,
	}
	config := &Config{
		CertConfig: certConfig,
	}
	_, err := InboundBuilder(config, nodeInfo, "test_tag", nil)
	if err != nil {
		t.Error(err)
	}
}

func TestBuildSS(t *testing.T) {
	nodeInfo := &api.NodeInfo{
		NodeType:          "Shadowsocks",
		NodeID:            1,
		Port:              1145,
		SpeedLimit:        0,
		AlterID:           2,
		TransportProtocol: "tcp",
		Host:              "test.test.tk",
		Path:              "v2ray",
		EnableTLS:         false,
		CypherMethod:      "aes-256-gcm",
	}
	DNSEnv := make(map[string]string)
	DNSEnv["ALICLOUD_ACCESS_KEY"] = "aaa"
	DNSEnv["ALICLOUD_SECRET_KEY"] = "bbb"
	certConfig := &mylego.CertConfig{
		CertMode:   "dns",
		CertDomain: "trojan.test.tk",
		Provider:   "alidns",
		Email:      "test@me.com",
		DNSEnv:     DNSEnv,
	}
	config := &Config{
		CertConfig: certConfig,
	}
	_, err := InboundBuilder(config, nodeInfo, "test_tag", nil)
	if err != nil {
		t.Error(err)
	}
}

func TestBuildSS2022WithUsers(t *testing.T) {
	key := func(b byte) string {
		return base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{b}, 32))
	}
	nodeInfo := &api.NodeInfo{
		NodeType:          "Shadowsocks",
		NodeID:            1,
		Port:              1145,
		TransportProtocol: "tcp",
		CypherMethod:      "2022-blake3-aes-256-gcm",
		ServerKey:         key(1),
	}
	config := &Config{}
	users := []*conf.ShadowsocksUserConfig{
		{Password: key(2), Email: "test_tag|a@example.com|1"},
	}
	_, err := InboundBuilder(config, nodeInfo, "test_tag", users)
	if err != nil {
		t.Error(err)
	}
}
