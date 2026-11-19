// Package seed loads idempotent demo data: two workspaces with a small
// catalogue each, for demonstrating the buyer and sales flows.
//
// Everything here is invented sample data. There is no real customer data,
// and the seed never touches order requests or orders, so it cannot interfere
// with the accept-flow tests.
package seed

// Product is one catalogue entry in the demo data.
type Product struct {
	Name string
	// Price is a decimal string, parsed into NUMERIC(12,2). Deliberately not
	// a float: money never passes through binary floating point.
	Price    string
	Currency string
	Stock    int32
}

// Workspace is a demo tenant and its catalogue.
type Workspace struct {
	Name string
	// Slug is stable across runs and is what makes the seed idempotent: it is
	// the conflict target when upserting the workspace.
	Slug     string
	Products []Product
}

// Demo is the data the seed command loads. Each workspace uses a single
// currency, because a request must have one currency for its total to be
// meaningful.
var Demo = []Workspace{
	{
		Name: "Lumière Paris",
		Slug: "lumiere-paris",
		Products: []Product{
			{Name: "Rose Absolute Serum 30ml", Price: "68.00", Currency: "EUR", Stock: 120},
			{Name: "Hydrating Rice Cleanser 150ml", Price: "32.50", Currency: "EUR", Stock: 240},
			{Name: "Night Repair Cream 50ml", Price: "89.90", Currency: "EUR", Stock: 65},
			{Name: "Vitamin C Eye Concentrate 15ml", Price: "54.00", Currency: "EUR", Stock: 48},
			{Name: "Silk Finish Setting Mist 100ml", Price: "27.00", Currency: "EUR", Stock: 310},
		},
	},
	{
		Name: "Coastal Botanics",
		Slug: "coastal-botanics",
		Products: []Product{
			{Name: "Sea Mineral Clay Mask 75ml", Price: "24.00", Currency: "USD", Stock: 180},
			{Name: "Kelp Recovery Hair Oil 100ml", Price: "38.00", Currency: "USD", Stock: 95},
			{Name: "Driftwood Body Scrub 200g", Price: "19.50", Currency: "USD", Stock: 260},
			{Name: "Tidal Glow Day Lotion SPF30", Price: "42.00", Currency: "USD", Stock: 140},
		},
	},
}
