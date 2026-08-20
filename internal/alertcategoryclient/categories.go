package alertcategoryclient

// categories is the ordered registry of alert categories. Adding a category is
// one entry here plus its queries/<id>.promql file — no new method or type.
//
// #1/#2 are broad (all firing|pending alerts, node-scoped vs domain-level);
// #3–#7 are curated per failure class with alertname allowlists and severity.
var categories = []CategorySpec{
	{ID: "node_alert_history", Title: "Alert History for Nodes in NVLink Domain", queryFile: "node_alert_history.promql"},
	{ID: "other_alerts", Title: "Other Alerts in the NVLink Domain", queryFile: "other_alerts.promql"},
	{ID: "node_nvlink_alerts", Title: "Node NVLink and GPU Alerts", queryFile: "node_nvlink_alerts.promql"},
	{ID: "node_pcie_alerts", Title: "Node PCI and PCIe Alerts", queryFile: "node_pcie_alerts.promql"},
	{ID: "node_redfish_alerts", Title: "Node Redfish and BMC Alerts", queryFile: "node_redfish_alerts.promql"},
	{ID: "nvlink_domain_alerts", Title: "NVLink Domain Alerts", queryFile: "nvlink_domain_alerts.promql"},
	{ID: "nvlink_switch_alerts", Title: "NVLink Switch Alerts", queryFile: "nvlink_switch_alerts.promql"},
}

// Categories returns a copy of the category registry (ids + titles), order
// stable. Both client backends delegate their Categories() method to this.
func Categories() []CategorySpec {
	out := make([]CategorySpec, len(categories))
	copy(out, categories)
	return out
}

// specByID looks up a category by id.
func specByID(id string) (CategorySpec, bool) {
	for _, c := range categories {
		if c.ID == id {
			return c, true
		}
	}
	return CategorySpec{}, false
}
