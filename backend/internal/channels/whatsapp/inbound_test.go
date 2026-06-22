package whatsapp

import "testing"

// textMessagePayload is a realistic Cloud API inbound text webhook body.
const textMessagePayload = `{
  "object": "whatsapp_business_account",
  "entry": [{
    "id": "WABA_ID",
    "changes": [{
      "field": "messages",
      "value": {
        "messaging_product": "whatsapp",
        "metadata": {"display_phone_number": "15550001111", "phone_number_id": "PNID"},
        "contacts": [{"profile": {"name": "Ana"}, "wa_id": "40712345678"}],
        "messages": [{
          "from": "40712345678",
          "id": "wamid.ABC123",
          "timestamp": "1700000000",
          "type": "text",
          "text": {"body": "Vreau 10 paleti de zahar"}
        }]
      }
    }]
  }]
}`

// statusCallbackPayload is a delivery-status callback (no messages[]).
const statusCallbackPayload = `{
  "object": "whatsapp_business_account",
  "entry": [{
    "changes": [{
      "field": "messages",
      "value": {
        "messaging_product": "whatsapp",
        "statuses": [{"id": "wamid.ABC123", "status": "delivered", "recipient_id": "40712345678"}]
      }
    }]
  }]
}`

func TestParseInbound_TextMessage(t *testing.T) {
	msgs, err := ParseInbound([]byte(textMessagePayload))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	m := msgs[0]
	if m.From != "40712345678" || m.ID != "wamid.ABC123" || m.Text != "Vreau 10 paleti de zahar" {
		t.Fatalf("unexpected parsed message: %+v", m)
	}
}

func TestParseInbound_StatusCallbackIgnored(t *testing.T) {
	msgs, err := ParseInbound([]byte(statusCallbackPayload))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(msgs) != 0 {
		t.Fatalf("status callback should yield no messages, got %d", len(msgs))
	}
}

func TestParseInbound_NonTextIgnored(t *testing.T) {
	payload := `{"entry":[{"changes":[{"value":{"messages":[
		{"from":"40712345678","id":"wamid.IMG","type":"image","image":{"id":"media1"}}
	]}}]}]}`
	msgs, err := ParseInbound([]byte(payload))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(msgs) != 0 {
		t.Fatalf("non-text message should be ignored, got %d", len(msgs))
	}
}

func TestParseInbound_MalformedJSON(t *testing.T) {
	if _, err := ParseInbound([]byte(`{not json`)); err == nil {
		t.Fatal("expected parse error for malformed JSON")
	}
}

func TestParseInbound_MultipleMessages(t *testing.T) {
	payload := `{"entry":[{"changes":[{"value":{"messages":[
		{"from":"4071","id":"wamid.1","type":"text","text":{"body":"unu"}},
		{"from":"4072","id":"wamid.2","type":"text","text":{"body":"doi"}}
	]}}]}]}`
	msgs, err := ParseInbound([]byte(payload))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
}
