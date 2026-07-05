package email

// routeFor maps a recipient's local part to a route name and its task template
// (docs/04). Route configuration is model state edited from the web UI in stage
// 3; for Stage 1 the small set is fixed here, and anything unrecognised falls
// through to the default "main" route rather than being rejected.
func routeFor(localPart string) (route, template string) {
	switch localPart {
	case "pay":
		return "pay", "invoice-payment"
	case "research":
		return "research", "research"
	default:
		return "main", "main"
	}
}
