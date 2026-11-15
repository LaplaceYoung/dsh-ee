package api

import (
	"testing"

	"github.com/hikariming/dsh-ee/server/internal/ldapauth"
)

// TestLDAPTestTargetChanged 覆盖「测试连接」判定中哪些字段算换了目标：
// 只有会改变密码发往何处、以谁的身份、经什么信道的字段才算；与连接无关的字段不算。
func TestLDAPTestTargetChanged(t *testing.T) {
	stored := func() *ldapauth.Config {
		return &ldapauth.Config{
			URL:                "ldap://dc.acme.local:389",
			BindDN:             "cn=svc,dc=acme,dc=local",
			StartTLS:           false,
			InsecureSkipVerify: false,
			CACert:             "-----BEGIN CERTIFICATE-----\r\nYWJj\r\n-----END CERTIFICATE-----\r\n",
			BaseDN:             "dc=acme,dc=local",
			UserFilter:         "(&(objectClass=person)(uid={username}))",
			TimeoutSeconds:     10,
			Enabled:            true,
		}
	}
	cases := []struct {
		name   string
		mutate func(*ldapauth.Config)
		want   bool
	}{
		{"完全相同", func(*ldapauth.Config) {}, false},
		{"主机名大小写与 scheme 大小写归一", func(c *ldapauth.Config) { c.URL = "LDAP://DC.ACME.LOCAL:389" }, false},
		{"省略默认端口", func(c *ldapauth.Config) { c.URL = "ldap://dc.acme.local" }, false},
		{"path 为 / 与空等价", func(c *ldapauth.Config) { c.URL = "ldap://dc.acme.local:389/" }, false},
		{"query 不决定连接目标", func(c *ldapauth.Config) { c.URL = "ldap://dc.acme.local:389?x=1" }, false},
		{"换了端口", func(c *ldapauth.Config) { c.URL = "ldap://dc.acme.local:1389" }, true},
		{"换了主机", func(c *ldapauth.Config) { c.URL = "ldap://attacker.example:389" }, true},
		{"换了 scheme", func(c *ldapauth.Config) { c.URL = "ldaps://dc.acme.local:389" }, true},
		{"换了 BindDN", func(c *ldapauth.Config) { c.BindDN = "cn=other,dc=acme,dc=local" }, true},
		{"开了 StartTLS", func(c *ldapauth.Config) { c.StartTLS = true }, true},
		{"关了证书校验", func(c *ldapauth.Config) { c.InsecureSkipVerify = true }, true},
		{"CA 只是换行风格", func(c *ldapauth.Config) { c.CACert = "-----BEGIN CERTIFICATE-----\nYWJj\n-----END CERTIFICATE-----" }, false},
		{"换了 CA", func(c *ldapauth.Config) { c.CACert = "-----BEGIN CERTIFICATE-----\nZGVm\n-----END CERTIFICATE-----" }, true},
		{"换了搜索起点", func(c *ldapauth.Config) { c.BaseDN = "dc=other,dc=local" }, true},
		{"只改过滤条件与超时", func(c *ldapauth.Config) {
			c.UserFilter = "(uid={username})"
			c.TimeoutSeconds = 3
			c.Enabled = false
		}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cur := stored()
			req := stored()
			tc.mutate(req)
			if got := ldapTestTargetChanged(req, cur); got != tc.want {
				t.Fatalf("ldapTestTargetChanged = %v，期望 %v", got, tc.want)
			}
		})
	}
}
