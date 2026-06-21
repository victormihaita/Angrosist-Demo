// Package email holds the Mailer adapters (port: ports.Mailer) and the in-code
// transactional templates. Templates are keyed by (name, locale) and rendered
// with Go's html/template (body) + text/template (subject and plain-text part),
// so the inner layers only ever pass a domain.EmailMessage and never build a
// subject or body string themselves.
//
// Templates currently live in code (this file) rather than the `templates` DB
// table (migration 023): for the three Phase-1/M3 emails this is simpler, has no
// runtime DB dependency, and keeps rendering deterministic and testable. A
// later iteration can load bodies from the table behind the same Mailer port
// without changing any call site.
package email

import (
	"bytes"
	"fmt"
	"html/template"
	"strings"
	texttemplate "text/template"

	"github.com/angrosist/demo/internal/domain"
)

// rendered is the concrete output of rendering a template: the wire-level email
// the adapters deliver. It is internal to this package — the port speaks
// domain.EmailMessage (name + locale + data), not rendered strings.
type rendered struct {
	Subject string
	HTML    string
	Text    string
}

// tmplSet is one template's three parts for a single locale.
type tmplSet struct {
	subject *texttemplate.Template
	html    *template.Template
	text    *texttemplate.Template
}

// templates is the compiled registry keyed by name then locale. It is built once
// at package init; a parse error there is a programming error and panics.
var templates = buildTemplates()

// defaultLocale is used when a requested locale has no variant for a template.
const defaultLocale = domain.LocaleRO

// render resolves (name, locale) — falling back to defaultLocale — and executes
// the three parts against data. It returns an error only for an unknown template
// name or a template-execution failure (a missing key renders empty, not error,
// thanks to the explicit field access in the templates).
func render(name, locale string, data map[string]any) (rendered, error) {
	byLocale, ok := templates[name]
	if !ok {
		return rendered{}, fmt.Errorf("email: unknown template %q", name)
	}
	set, ok := byLocale[locale]
	if !ok {
		set = byLocale[defaultLocale]
	}
	if data == nil {
		data = map[string]any{}
	}

	var subj, htmlBuf, textBuf bytes.Buffer
	if err := set.subject.Execute(&subj, data); err != nil {
		return rendered{}, fmt.Errorf("email: render subject %q/%s: %w", name, locale, err)
	}
	if err := set.html.Execute(&htmlBuf, data); err != nil {
		return rendered{}, fmt.Errorf("email: render html %q/%s: %w", name, locale, err)
	}
	if err := set.text.Execute(&textBuf, data); err != nil {
		return rendered{}, fmt.Errorf("email: render text %q/%s: %w", name, locale, err)
	}
	return rendered{
		Subject: strings.TrimSpace(subj.String()),
		HTML:    htmlBuf.String(),
		Text:    strings.TrimSpace(textBuf.String()),
	}, nil
}

// resolveLocale normalizes a requested locale to one we support, defaulting to
// RO. Exposed for the adapters' logging.
func resolveLocale(locale string) string {
	switch strings.ToLower(strings.TrimSpace(locale)) {
	case domain.LocaleEN:
		return domain.LocaleEN
	default:
		return domain.LocaleRO
	}
}

func mustText(name, src string) *texttemplate.Template {
	return texttemplate.Must(texttemplate.New(name).Parse(src))
}

func mustHTML(name, src string) *template.Template {
	return template.Must(template.New(name).Parse(src))
}

// buildTemplates compiles every (name, locale) variant. The bodies are
// intentionally short and use only fields supplied by the agent/use-cases
// (.Product, .Quantity, .Unit, .Company, .ContactName, .LeadID, .Reason,
// .Summary, .ConversationID).
func buildTemplates() map[string]map[string]tmplSet {
	return map[string]map[string]tmplSet{
		domain.EmailLeadConfirmation: {
			domain.LocaleRO: {
				subject: mustText("conf.ro.subj", `Cererea ta a fost înregistrată — Euro Intermed`),
				html: mustHTML("conf.ro.html", `<p>Bună{{if .ContactName}} {{.ContactName}}{{end}},</p>
<p>Am primit cererea ta și echipa Euro Intermed te va contacta în curând.</p>
<p><strong>Rezumat cerere:</strong></p>
<ul>
<li>Produs: {{.Product}}</li>
<li>Cantitate: {{.Quantity}}{{if .Unit}} {{.Unit}}{{end}}</li>
{{if .Company}}<li>Companie: {{.Company}}</li>{{end}}
</ul>
<p>Mulțumim,<br>Echipa Euro Intermed</p>`),
				text: mustText("conf.ro.text", `Bună{{if .ContactName}} {{.ContactName}}{{end}},

Am primit cererea ta și echipa Euro Intermed te va contacta în curând.

Rezumat cerere:
- Produs: {{.Product}}
- Cantitate: {{.Quantity}}{{if .Unit}} {{.Unit}}{{end}}
{{if .Company}}- Companie: {{.Company}}{{end}}

Mulțumim,
Echipa Euro Intermed`),
			},
			domain.LocaleEN: {
				subject: mustText("conf.en.subj", `We received your request — Euro Intermed`),
				html: mustHTML("conf.en.html", `<p>Hello{{if .ContactName}} {{.ContactName}}{{end}},</p>
<p>We received your request and the Euro Intermed team will contact you shortly.</p>
<p><strong>Request summary:</strong></p>
<ul>
<li>Product: {{.Product}}</li>
<li>Quantity: {{.Quantity}}{{if .Unit}} {{.Unit}}{{end}}</li>
{{if .Company}}<li>Company: {{.Company}}</li>{{end}}
</ul>
<p>Thank you,<br>The Euro Intermed team</p>`),
				text: mustText("conf.en.text", `Hello{{if .ContactName}} {{.ContactName}}{{end}},

We received your request and the Euro Intermed team will contact you shortly.

Request summary:
- Product: {{.Product}}
- Quantity: {{.Quantity}}{{if .Unit}} {{.Unit}}{{end}}
{{if .Company}}- Company: {{.Company}}{{end}}

Thank you,
The Euro Intermed team`),
			},
		},

		domain.EmailStaffLeadNotification: {
			domain.LocaleRO: {
				subject: mustText("staff.ro.subj", `Lead nou calificat: {{.Company}}`),
				html: mustHTML("staff.ro.html", `<p>A sosit un lead nou calificat.</p>
<ul>
<li>Companie: {{.Company}}{{if .CUI}} (CUI {{.CUI}}){{end}}</li>
<li>Produs: {{.Product}}</li>
<li>Cantitate: {{.Quantity}}{{if .Unit}} {{.Unit}}{{end}}</li>
<li>Locație livrare: {{.DeliveryLocation}}</li>
<li>Contact: {{.Phone}}{{if .Email}} / {{.Email}}{{end}}</li>
<li>Lead ID: {{.LeadID}}</li>
</ul>`),
				text: mustText("staff.ro.text", `A sosit un lead nou calificat.

Companie: {{.Company}}{{if .CUI}} (CUI {{.CUI}}){{end}}
Produs: {{.Product}}
Cantitate: {{.Quantity}}{{if .Unit}} {{.Unit}}{{end}}
Locație livrare: {{.DeliveryLocation}}
Contact: {{.Phone}}{{if .Email}} / {{.Email}}{{end}}
Lead ID: {{.LeadID}}`),
			},
			domain.LocaleEN: {
				subject: mustText("staff.en.subj", `New qualified lead: {{.Company}}`),
				html: mustHTML("staff.en.html", `<p>A new qualified lead has arrived.</p>
<ul>
<li>Company: {{.Company}}{{if .CUI}} (reg {{.CUI}}){{end}}</li>
<li>Product: {{.Product}}</li>
<li>Quantity: {{.Quantity}}{{if .Unit}} {{.Unit}}{{end}}</li>
<li>Delivery location: {{.DeliveryLocation}}</li>
<li>Contact: {{.Phone}}{{if .Email}} / {{.Email}}{{end}}</li>
<li>Lead ID: {{.LeadID}}</li>
</ul>`),
				text: mustText("staff.en.text", `A new qualified lead has arrived.

Company: {{.Company}}{{if .CUI}} (reg {{.CUI}}){{end}}
Product: {{.Product}}
Quantity: {{.Quantity}}{{if .Unit}} {{.Unit}}{{end}}
Delivery location: {{.DeliveryLocation}}
Contact: {{.Phone}}{{if .Email}} / {{.Email}}{{end}}
Lead ID: {{.LeadID}}`),
			},
		},

		domain.EmailHandoffNotification: {
			domain.LocaleRO: {
				subject: mustText("handoff.ro.subj", `Conversație escaladată — necesită un coleg`),
				html: mustHTML("handoff.ro.html", `<p>O conversație necesită intervenția unui coleg.</p>
<ul>
<li>Motiv: {{.Reason}}</li>
{{if .Summary}}<li>Rezumat: {{.Summary}}</li>{{end}}
<li>Conversație ID: {{.ConversationID}}</li>
{{if .LeadID}}<li>Lead ID: {{.LeadID}}</li>{{end}}
</ul>`),
				text: mustText("handoff.ro.text", `O conversație necesită intervenția unui coleg.

Motiv: {{.Reason}}
{{if .Summary}}Rezumat: {{.Summary}}{{end}}
Conversație ID: {{.ConversationID}}
{{if .LeadID}}Lead ID: {{.LeadID}}{{end}}`),
			},
			domain.LocaleEN: {
				subject: mustText("handoff.en.subj", `Conversation escalated — needs a human`),
				html: mustHTML("handoff.en.html", `<p>A conversation needs a human.</p>
<ul>
<li>Reason: {{.Reason}}</li>
{{if .Summary}}<li>Summary: {{.Summary}}</li>{{end}}
<li>Conversation ID: {{.ConversationID}}</li>
{{if .LeadID}}<li>Lead ID: {{.LeadID}}</li>{{end}}
</ul>`),
				text: mustText("handoff.en.text", `A conversation needs a human.

Reason: {{.Reason}}
{{if .Summary}}Summary: {{.Summary}}{{end}}
Conversation ID: {{.ConversationID}}
{{if .LeadID}}Lead ID: {{.LeadID}}{{end}}`),
			},
		},
	}
}
