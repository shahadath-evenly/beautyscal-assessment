package email_test

import (
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/real-musafir/b-scale/backend/internal/email"
)

func acceptedFixture() email.AcceptedRequest {
	return email.AcceptedRequest{
		RequestID:  uuid.MustParse("3f2a1b4c-5d6e-4f70-8192-a3b4c5d6e7f8"),
		BuyerName:  "Ada Lovelace",
		BuyerEmail: "ada@example.test",
		BrandName:  "Lumière Paris",
		Currency:   "EUR",
		Total:      "225.90",

		Lines: []email.Line{
			{ProductName: "Rose Absolute Serum 30ml", Quantity: 2, UnitPrice: "68.00", LineTotal: "136.00"},
			{ProductName: "Night Repair Cream 50ml", Quantity: 1, UnitPrice: "89.90", LineTotal: "89.90"},
		},
	}
}

// The message must carry everything the buyer needs to recognise what was
// accepted: who they are, which request, what was in it and what it costs.
func TestRenderAcceptedIncludesEveryRequiredDetail(t *testing.T) {
	msg := email.RenderAccepted(acceptedFixture())

	if msg.ToEmail != "ada@example.test" || msg.ToName != "Ada Lovelace" {
		t.Fatalf("recipient = %q <%q>", msg.ToName, msg.ToEmail)
	}
	if !strings.Contains(msg.Subject, "accepted") {
		t.Errorf("subject %q does not say the request was accepted", msg.Subject)
	}

	for _, body := range []struct {
		name string
		text string
	}{{"text", msg.Text}, {"html", msg.HTML}} {
		for _, want := range []string{
			"Ada Lovelace",
			"3f2a1b4c-5d6e-4f70-8192-a3b4c5d6e7f8",
			"Rose Absolute Serum 30ml",
			"Night Repair Cream 50ml",
			"68.00",
			"136.00",
			"225.90",
			"EUR",
			"Lumière Paris",
		} {
			if !strings.Contains(body.text, want) {
				t.Errorf("%s body is missing %q", body.name, want)
			}
		}
		// Quantities, which are rendered as bare numbers.
		if !strings.Contains(body.text, "2") || !strings.Contains(body.text, "1") {
			t.Errorf("%s body is missing the quantities", body.name)
		}
	}
}

// A product name is brand-supplied data appearing in an HTML document, so it
// must be escaped rather than interpolated.
func TestRenderAcceptedEscapesHTML(t *testing.T) {
	req := acceptedFixture()
	req.BuyerName = `Ada <script>alert("x")</script>`
	req.Lines[0].ProductName = `Serum & <b>Oil</b>`

	msg := email.RenderAccepted(req)

	if strings.Contains(msg.HTML, "<script>") {
		t.Error("HTML body contains an unescaped script tag")
	}
	if !strings.Contains(msg.HTML, "&lt;script&gt;") {
		t.Error("HTML body did not escape the buyer name")
	}
	if !strings.Contains(msg.HTML, "Serum &amp; &lt;b&gt;Oil&lt;/b&gt;") {
		t.Error("HTML body did not escape the product name")
	}
	// The text part is not markup and must stay readable as written.
	if !strings.Contains(msg.Text, `Serum & <b>Oil</b>`) {
		t.Error("text body should keep the raw product name")
	}
}

func TestRenderAcceptedFallsBackWhenBrandIsBlank(t *testing.T) {
	req := acceptedFixture()
	req.BrandName = "   "

	msg := email.RenderAccepted(req)
	if !strings.Contains(msg.Text, "the brand") {
		t.Errorf("expected a fallback brand name, got %q", msg.Text)
	}
}
