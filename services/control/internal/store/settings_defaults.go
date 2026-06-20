package store

import "time"

const (
	defaultSystemName               = "LingCDN"
	defaultFooterCopyright          = "Copyright {year} {name}. All rights reserved."
	defaultSalesEmail               = "sales@lingcdn.cloud"
	defaultSupportEmail             = "support@lingcdn.cloud"
	defaultSMTPPort                 = 587
	defaultElasticsearchIndex       = "cdn-access"
	defaultElasticsearchTSField     = "@timestamp"
	defaultElasticsearchDomainField = "domain"
	defaultElasticsearchBytesField  = "bytes"
	defaultUpgradeChannel           = "stable"
	defaultNotifyInterval           = 5
	defaultRenewalBeforeExpiryDays  = 30
	defaultRetentionSystemLogs      = 90
	defaultRetentionESLogs          = 7
	defaultRetentionWafBans         = 7
	defaultRetentionUpgradeLogs     = 30
)

// DefaultSettings returns the system defaults used for a fresh install.
func DefaultSettings() *Settings {
	return &Settings{
		ID:                        "default",
		SystemName:                defaultSystemName,
		FooterLinks:               "",
		FooterCopyright:           defaultFooterCopyright,
		Favicon:                   "",
		Logo:                      "",
		SidebarBrandMode:          "name",
		SMTPHost:                  "",
		SMTPPort:                  defaultSMTPPort,
		SMTPUsername:              "",
		SMTPPassword:              "",
		SMTPFrom:                  "",
		SMTPFromName:              defaultSystemName,
		ElasticsearchURL:          "",
		ElasticsearchUser:         "",
		ElasticsearchPass:         "",
		ElasticsearchIndex:        defaultElasticsearchIndex,
		ElasticsearchTSField:      defaultElasticsearchTSField,
		ElasticsearchDomainField:  defaultElasticsearchDomainField,
		ElasticsearchBytesField:   defaultElasticsearchBytesField,
		SalesEmail:                defaultSalesEmail,
		SupportEmail:              defaultSupportEmail,
		RegisterEnabled:           true,
		UpgradeChannel:            defaultUpgradeChannel,
		NotifyNewBuild:            true,
		RegisterEmailVerification: false,
		RenewalBeforeExpiryDays:   defaultRenewalBeforeExpiryDays,
		EmailEnabled:              false,
		DingtalkEnabled:           false,
		DingtalkWebhook:           "",
		WechatEnabled:             false,
		WechatWebhook:             "",
		FeishuEnabled:             false,
		FeishuWebhook:             "",
		NotifyNodeResource:        false,
		NotifyNodeMonitor:         false,
		NotifyTicketReply:         false,
		NotifyInterval:            defaultNotifyInterval,
		ThresholdCPU:              0,
		ThresholdMemory:           0,
		ThresholdDisk:             0,
		ThresholdBandwidthUp:      0,
		ThresholdBandwidthDown:    0,
		RetentionSystemLogs:       defaultRetentionSystemLogs,
		RetentionESLogs:           defaultRetentionESLogs,
		RetentionWafBans:          defaultRetentionWafBans,
		RetentionUpgradeLogs:      defaultRetentionUpgradeLogs,
		UpdatedAt:                 time.Now(),
	}
}
