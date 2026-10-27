package alerts

import (
	"bytes"
	"crypto/rand"
	_ "embed"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"html/template"
	"mime"
	"mime/quotedprintable"
	"net/mail"
	"strings"
	"time"
)

//go:embed brand-mark.png
var logo []byte

type item struct{ Key, Kind, Repository, Title, Detail, URL, Action string }

type group struct {
	Label, Colour string
	Items         []item
}

type email struct {
	Subject, Heading, Intro, Org, CTA, CTAURL, Manage string
	Groups                                            []group
}

var groupOrder = []struct{ Kind, Label, Colour string }{
	{"person", "Needs you", "#b45309"},
	{"review", "Awaiting review", "#2458d8"},
	{"paused", "Paused runs", "#6b7280"},
	{"blocked", "Blocked", "#b91c1c"},
}

func digest(base, org, name string, items []item) email {
	e := email{Org: name, CTA: "Open Reforge", CTAURL: base + "/org/" + org, Manage: base + "/org/" + org + "/organisation"}
	e.Heading = fmt.Sprintf("%d items need attention", len(items))
	if len(items) == 1 {
		e.Heading = "1 item needs attention"
	}
	e.Subject = "Reforge: " + strings.ToLower(e.Heading[:1]) + e.Heading[1:]
	e.Intro = "Autopilot has paused on the following. Everything else continues automatically."
	for _, g := range groupOrder {
		out := group{Label: g.Label, Colour: g.Colour}
		for _, it := range items {
			if it.Detail != "" {
				it.Detail = strings.ToUpper(it.Detail[:1]) + it.Detail[1:]
			}
			if it.Kind == g.Kind {
				out.Items = append(out.Items, it)
			}
		}
		if len(out.Items) > 0 {
			e.Groups = append(e.Groups, out)
		}
	}
	return e
}

func (e email) text() string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s\n%s\n", e.Heading, e.Intro)
	for _, g := range e.Groups {
		fmt.Fprintf(&b, "\n%s\n", strings.ToUpper(g.Label))
		for _, it := range g.Items {
			fmt.Fprintf(&b, "- %s: %s\n", it.Repository, it.Title)
			if it.Detail != "" {
				fmt.Fprintf(&b, "  %s\n", it.Detail)
			}
			if it.URL != "" {
				fmt.Fprintf(&b, "  %s: %s\n", it.Action, it.URL)
			}
		}
	}
	fmt.Fprintf(&b, "\n%s: %s\n", e.CTA, e.CTAURL)
	if e.Manage != "" {
		fmt.Fprintf(&b, "Manage alerts: %s\n", e.Manage)
	}
	return b.String()
}

func (e email) html() (string, error) {
	var b bytes.Buffer
	err := page.Execute(&b, e)
	return b.String(), err
}

var page = template.Must(template.New("email").Parse(`<!doctype html>
<html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><meta name="color-scheme" content="light"><title>{{.Subject}}</title></head>
<body style="margin:0;padding:0;background:#f4f5f7;font-family:Inter,-apple-system,'Segoe UI',Roboto,Helvetica,Arial,sans-serif;color:#111827;">
<table role="presentation" width="100%" cellspacing="0" cellpadding="0" style="background:#f4f5f7;"><tr><td align="center" style="padding:32px 16px;">
<table role="presentation" width="600" cellspacing="0" cellpadding="0" style="width:100%;max-width:600px;">
<tr><td style="padding:0 4px 16px;">
<table role="presentation" width="100%" cellspacing="0" cellpadding="0"><tr>
<td style="vertical-align:middle;"><img src="cid:logo@reforge" width="28" height="28" alt="" style="display:inline-block;vertical-align:middle;border:0;"><span style="display:inline-block;vertical-align:middle;margin-left:10px;font-size:18px;font-weight:700;letter-spacing:-0.2px;color:#111827;">Reforge</span></td>
{{if .Org}}<td align="right" style="vertical-align:middle;font-size:13px;color:#6b7280;">{{.Org}}</td>{{end}}
</tr></table></td></tr>
<tr><td style="background:#ffffff;border:1px solid #e2e5eb;border-top:3px solid #2458d8;border-radius:8px;padding:28px 32px;">
<h1 style="margin:0 0 6px;font-size:20px;line-height:28px;font-weight:650;color:#111827;">{{.Heading}}</h1>
<p style="margin:0 0 20px;font-size:14px;line-height:21px;color:#4b5563;">{{.Intro}}</p>
{{range .Groups}}
<p style="margin:20px 0 8px;font-size:11px;font-weight:700;letter-spacing:0.6px;text-transform:uppercase;color:{{.Colour}};">{{.Label}}</p>
{{$colour := .Colour}}{{range .Items}}
<table role="presentation" width="100%" cellspacing="0" cellpadding="0" style="margin:0 0 10px;border:1px solid #e2e5eb;border-left:3px solid {{$colour}};border-radius:6px;background:#f8f9fb;"><tr>
<td style="padding:12px 14px;">
<div style="font-size:12px;line-height:16px;color:#6b7280;">{{.Repository}}</div>
<div style="margin-top:2px;font-size:14px;line-height:20px;font-weight:600;color:#111827;">{{.Title}}</div>
{{if .Detail}}<div style="margin-top:4px;font-size:13px;line-height:19px;color:#4b5563;">{{.Detail}}</div>{{end}}
</td>
{{if .URL}}<td align="right" style="padding:12px 14px 12px 0;vertical-align:middle;white-space:nowrap;"><a href="{{.URL}}" style="display:inline-block;padding:7px 12px;border:1px solid #d1d5db;border-radius:6px;background:#ffffff;font-size:13px;font-weight:600;color:#2458d8;text-decoration:none;">{{.Action}}</a></td>{{end}}
</tr></table>
{{end}}{{end}}
<table role="presentation" cellspacing="0" cellpadding="0" style="margin-top:24px;"><tr><td style="border-radius:6px;background:#2458d8;"><a href="{{.CTAURL}}" style="display:inline-block;padding:10px 18px;font-size:14px;font-weight:600;color:#ffffff;text-decoration:none;">{{.CTA}}</a></td></tr></table>
</td></tr>
{{if .Manage}}<tr><td style="padding:16px 4px;font-size:12px;line-height:18px;color:#6b7280;">You receive this as an alert recipient{{if .Org}} for {{.Org}}{{end}}. <a href="{{.Manage}}" style="color:#6b7280;text-decoration:underline;">Manage alerts</a></td></tr>{{end}}
</table></td></tr></table>
</body></html>`))

func compose(from string, to []string, subject, text, html string) []byte {
	var raw [12]byte
	_, _ = rand.Read(raw[:])
	boundary := "reforge-" + hex.EncodeToString(raw[:])
	var b bytes.Buffer
	sender := &mail.Address{Name: "Reforge", Address: from}
	if parsed, err := mail.ParseAddress(from); err == nil {
		sender.Address = parsed.Address
		if parsed.Name != "" {
			sender.Name = parsed.Name
		}
	}
	fmt.Fprintf(&b, "From: %s\r\nTo: %s\r\nSubject: %s\r\nDate: %s\r\nMIME-Version: 1.0\r\nContent-Type: multipart/alternative; boundary=%q\r\n", sender.String(), strings.Join(to, ", "), mime.QEncoding.Encode("utf-8", subject), time.Now().UTC().Format(time.RFC1123Z), boundary)
	fmt.Fprintf(&b, "\r\n--%s\r\n", boundary)
	part(&b, "text/plain", text)
	related := boundary + "-related"
	fmt.Fprintf(&b, "\r\n--%s\r\nContent-Type: multipart/related; boundary=%q\r\n\r\n--%s\r\n", boundary, related, related)
	part(&b, "text/html", html)
	fmt.Fprintf(&b, "\r\n--%s\r\nContent-Type: image/png\r\nContent-Transfer-Encoding: base64\r\nContent-ID: <logo@reforge>\r\nContent-Disposition: inline; filename=\"reforge.png\"\r\n\r\n", related)
	encoded := base64.StdEncoding.EncodeToString(logo)
	for len(encoded) > 76 {
		b.WriteString(encoded[:76] + "\r\n")
		encoded = encoded[76:]
	}
	fmt.Fprintf(&b, "%s\r\n--%s--\r\n\r\n--%s--\r\n", encoded, related, boundary)
	return b.Bytes()
}

func part(b *bytes.Buffer, kind, body string) {
	fmt.Fprintf(b, "Content-Type: %s; charset=utf-8\r\nContent-Transfer-Encoding: quoted-printable\r\n\r\n", kind)
	w := quotedprintable.NewWriter(b)
	_, _ = w.Write([]byte(strings.ReplaceAll(body, "\n", "\r\n")))
	_ = w.Close()
}

func SendInvitation(server SMTP, to, orgName, link string) error {
	e := email{Org: orgName, CTA: "Accept invitation", CTAURL: link}
	e.Subject = "You're invited to Reforge"
	e.Heading = "Set up " + orgName + " on Reforge"
	e.Intro = "You've been invited to the Reforge beta. Accept to sign in and create your workspace; the link works once and expires in 7 days."
	return deliver(server, []string{to}, e)
}
