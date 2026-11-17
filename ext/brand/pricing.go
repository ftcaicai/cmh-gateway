package brand

import (
	"net/http"
	"slices"
	"strings"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
)

// storefrontGroup is the group whose published models and standard prices the
// brand storefront shows. Brand-specific prices do not exist yet (H4); a brand
// customer is billed by the official pricing of their own group.
const storefrontGroup = "default"

// storefrontModel is the public projection of one published model. It carries
// no channel, upstream, ratio or group data.
type storefrontModel struct {
	Name        string   `json:"name"`
	Vendor      string   `json:"vendor"`
	Description string   `json:"description"`
	Endpoints   []string `json:"endpoints"`
	// Billing is "token" (Input/Output per million tokens), "call" (Call per
	// request) or "rule" (priced by a billing rule that has no single figure).
	Billing  string  `json:"billing"`
	Currency string  `json:"currency"`
	Input    float64 `json:"input"`
	Output   float64 `json:"output"`
	Call     float64 `json:"call"`
}

// brandPricing lists the models published to storefrontGroup with their
// standard price. It is display only: the figures follow the official
// definition (ratio × 2 USD per million tokens × group ratio, or the per-call
// price × group ratio) and never feed back into billing.
func brandPricing(c *gin.Context) {
	if BrandID(c) == "" {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}
	// Tokens and custom units have no public per-token price; fall back to USD.
	currency, rate := "$", 1.0
	if operation_setting.GetQuotaDisplayType() == operation_setting.QuotaDisplayTypeCNY {
		currency, rate = "¥", operation_setting.USDExchangeRate
	}
	rate *= ratio_setting.GetGroupRatio(storefrontGroup)
	vendors := map[int]string{}
	for _, vendor := range model.GetVendors() {
		vendors[vendor.ID] = vendor.Name
	}
	models := []storefrontModel{}
	for _, pricing := range model.GetPricing() {
		if !slices.Contains(pricing.EnableGroup, storefrontGroup) && !slices.Contains(pricing.EnableGroup, "all") {
			continue
		}
		item := storefrontModel{
			Name: pricing.ModelName, Vendor: vendors[pricing.VendorID], Description: pricing.Description,
			Endpoints: []string{}, Billing: "rule", Currency: currency,
		}
		for _, endpoint := range pricing.SupportedEndpointTypes {
			item.Endpoints = append(item.Endpoints, string(endpoint))
		}
		switch {
		case pricing.BillingExpr != "" || len(pricing.BillingPluginVariants) > 0:
		case pricing.QuotaType == 0:
			item.Billing, item.Input = "token", pricing.ModelRatio*2*rate
			item.Output = item.Input * pricing.CompletionRatio
		case pricing.QuotaType == 1:
			item.Billing, item.Call = "call", pricing.ModelPrice*rate
		}
		models = append(models, item)
	}
	slices.SortFunc(models, func(a, b storefrontModel) int {
		if byVendor := strings.Compare(a.Vendor, b.Vendor); byVendor != 0 {
			return byVendor
		}
		return strings.Compare(a.Name, b.Name)
	})
	c.JSON(http.StatusOK, gin.H{"success": true, "data": models})
}
