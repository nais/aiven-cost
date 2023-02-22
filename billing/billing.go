package billing

import (
	"strconv"
	"time"
)

type AivenCostItem struct {
	BillingGroupId string
	InvoiceId   string
	Environment string
	Team        string
	StartDate   time.Time
	EndDate     time.Time
	Service     string
	Cost        float64
	Tenant      string
}

func (a *AivenCostItem) CostPerDay() float64 {
	return a.Cost / float64(a.NumberOfDays())
}

func (a *AivenCostItem) costToString() string {
	return strconv.FormatFloat(a.Cost, 'f', 2, 64)
}

func (a *AivenCostItem) String() string {
	return a.BillingGroupId + "," + a.InvoiceId + "," + a.Environment + "," + a.Team + "," + a.StartDate.String() + "," + a.EndDate.String() + "," + a.Service + "," + a.costToString() + "," + a.Tenant
}

func (a *AivenCostItem) NumberOfDays() int {
	return int(a.EndDate.Sub(a.StartDate).Hours()/24 + 1)
}
