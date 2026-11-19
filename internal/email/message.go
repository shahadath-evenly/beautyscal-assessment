// Package email turns an accepted order request into a message for the buyer
// and delivers it out of band.
//
// Nothing here runs inside an HTTP handler. Acceptance writes a row to the
// outbox in its own transaction and returns; a worker sends it afterwards. A
// slow, failing or unreachable email provider therefore cannot delay a
// response, and cannot roll back an order that has already reserved stock.
package email

import (
	"fmt"
	"html"
	"strings"

	"github.com/google/uuid"
)

// EventAccepted is the only event type the outbox carries today. The column
// is constrained to this value, so adding another means a migration.
const EventAccepted = "order_request_accepted"

// Message is a rendered email, ready to hand to a provider.
type Message struct {
	ToEmail string
	ToName  string
	Subject string
	Text    string
	HTML    string
}

// Line is one product on the accepted request.
type Line struct {
	ProductName string
	Quantity    int32
	UnitPrice   string
	LineTotal   string
}

// AcceptedRequest is everything the acceptance message needs. It is a plain
// value rather than a database row so rendering can be tested without a
// database, and so the email package does not depend on the order request
// module.
type AcceptedRequest struct {
	RequestID  uuid.UUID
	BuyerName  string
	BuyerEmail string
	Lines      []Line
	Total      string
	Currency   string
	BrandName  string
}

// RenderAccepted builds the acceptance email.
//
// Both a plain-text and an HTML body are produced: some clients show one,
// some the other, and a text part keeps the message legible where HTML is
// stripped. Every amount is a decimal string taken from the stored
// snapshots — no arithmetic happens here, so the buyer is told exactly what
// the acceptance recorded.
func RenderAccepted(req AcceptedRequest) Message {
	brand := strings.TrimSpace(req.BrandName)
	if brand == "" {
		brand = "the brand"
	}

	subject := fmt.Sprintf("Your order request has been accepted (%s)", shortRef(req.RequestID))

	var text strings.Builder
	fmt.Fprintf(&text, "Hello %s,\n\n", req.BuyerName)
	fmt.Fprintf(&text, "Good news: %s has accepted your order request.\n\n", brand)
	fmt.Fprintf(&text, "Reference: %s\n\n", req.RequestID)
	text.WriteString("What was accepted:\n")
	for _, line := range req.Lines {
		fmt.Fprintf(&text, "  - %s — %d x %s %s = %s %s\n",
			line.ProductName, line.Quantity, line.UnitPrice, req.Currency,
			line.LineTotal, req.Currency)
	}
	fmt.Fprintf(&text, "\nTotal: %s %s\n\n", req.Total, req.Currency)
	text.WriteString("A draft order has been created and the stock reserved.\n")
	text.WriteString("The team will be in touch with the next steps.\n")

	var body strings.Builder
	body.WriteString(`<!doctype html><html><body style="font-family:Arial,Helvetica,sans-serif;color:#171717;">`)
	fmt.Fprintf(&body, `<p>Hello %s,</p>`, html.EscapeString(req.BuyerName))
	fmt.Fprintf(&body, `<p>Good news: <strong>%s</strong> has accepted your order request.</p>`,
		html.EscapeString(brand))
	fmt.Fprintf(&body, `<p style="color:#737373;">Reference: %s</p>`, html.EscapeString(req.RequestID.String()))
	body.WriteString(`<table cellpadding="8" cellspacing="0" style="border-collapse:collapse;">`)
	body.WriteString(`<thead><tr>` +
		`<th align="left" style="border-bottom:1px solid #e5e5e5;">Product</th>` +
		`<th align="right" style="border-bottom:1px solid #e5e5e5;">Quantity</th>` +
		`<th align="right" style="border-bottom:1px solid #e5e5e5;">Unit price</th>` +
		`<th align="right" style="border-bottom:1px solid #e5e5e5;">Line total</th>` +
		`</tr></thead><tbody>`)
	for _, line := range req.Lines {
		fmt.Fprintf(&body,
			`<tr><td>%s</td><td align="right">%d</td><td align="right">%s %s</td><td align="right">%s %s</td></tr>`,
			html.EscapeString(line.ProductName), line.Quantity,
			html.EscapeString(line.UnitPrice), html.EscapeString(req.Currency),
			html.EscapeString(line.LineTotal), html.EscapeString(req.Currency))
	}
	body.WriteString(`</tbody><tfoot><tr>` +
		`<th align="right" colspan="3" style="border-top:2px solid #e5e5e5;">Total</th>`)
	fmt.Fprintf(&body, `<td align="right" style="border-top:2px solid #e5e5e5;"><strong>%s %s</strong></td>`,
		html.EscapeString(req.Total), html.EscapeString(req.Currency))
	body.WriteString(`</tr></tfoot></table>`)
	body.WriteString(`<p>A draft order has been created and the stock reserved. ` +
		`The team will be in touch with the next steps.</p>`)
	body.WriteString(`</body></html>`)

	return Message{
		ToEmail: req.BuyerEmail,
		ToName:  req.BuyerName,
		Subject: subject,
		Text:    text.String(),
		HTML:    body.String(),
	}
}

// shortRef is the first block of the UUID: enough for a buyer to quote in a
// subject line without pasting the whole reference.
func shortRef(id uuid.UUID) string {
	s := id.String()
	if len(s) >= 8 {
		return s[:8]
	}
	return s
}
