package mailer

import (
	"bytes"
	"crypto/tls"
	_ "embed"
	"encoding/base64"
	"fmt"
	"mime"
	"net"
	"net/mail"
	"net/smtp"
	"strings"

	"chatgpt2api/internal/config"
)

//go:embed assets/logo_icon.png
var logoIconPNG []byte

//go:embed assets/chibi.png
var chibiPNG []byte

//go:embed assets/character.png
var characterPNG []byte

type Sender struct {
	enabled        bool
	host           string
	port           int
	username       string
	password       string
	fromAddress    string
	fromName       string
	useImplicitTLS bool
}

func NewSender(cfg config.MailConfig) *Sender {
	fromAddress := strings.TrimSpace(cfg.FromAddress)
	username := firstNonEmpty(strings.TrimSpace(cfg.Username), fromAddress)
	host := strings.TrimSpace(cfg.SMTPHost)
	if host == "" {
		host = inferSMTPHost(firstNonEmpty(username, fromAddress))
	}
	port := cfg.SMTPPort
	if port <= 0 {
		if cfg.UseImplicitTLS {
			port = 465
		} else {
			port = 587
		}
	}

	return &Sender{
		enabled:        cfg.Enabled,
		host:           host,
		port:           port,
		username:       username,
		password:       strings.TrimSpace(cfg.Password),
		fromAddress:    fromAddress,
		fromName:       strings.TrimSpace(cfg.FromName),
		useImplicitTLS: cfg.UseImplicitTLS,
	}
}

func (s *Sender) Enabled() bool {
	return s != nil &&
		s.enabled &&
		s.host != "" &&
		s.port > 0 &&
		s.username != "" &&
		s.password != "" &&
		s.fromAddress != ""
}

func (s *Sender) SendVerificationCode(toAddress, code string) error {
	if !s.Enabled() {
		return fmt.Errorf("mail sender is not configured")
	}

	subject := mime.QEncoding.Encode("utf-8", "Cheilins Studio 验证码")
	fromHeader := s.fromAddress
	if strings.TrimSpace(s.fromName) != "" {
		fromHeader = (&mail.Address{
			Name:    s.fromName,
			Address: s.fromAddress,
		}).String()
	}

	body := buildVerificationCodeHTML(firstNonEmpty(s.fromName, "Cheilins Studio"), strings.TrimSpace(code))
	relatedBody, contentType, err := buildInlineHTMLMessage(body)
	if err != nil {
		return err
	}

	message := strings.Join([]string{
		"From: " + fromHeader,
		"To: " + strings.TrimSpace(toAddress),
		"Subject: " + subject,
		"MIME-Version: 1.0",
		"Content-Type: " + contentType,
		"",
		relatedBody,
	}, "\r\n")

	addr := net.JoinHostPort(s.host, fmt.Sprintf("%d", s.port))
	auth := smtp.PlainAuth("", s.username, s.password, s.host)

	if s.useImplicitTLS {
		conn, err := tls.Dial("tcp", addr, &tls.Config{ServerName: s.host})
		if err != nil {
			return err
		}
		defer conn.Close()

		client, err := smtp.NewClient(conn, s.host)
		if err != nil {
			return err
		}
		defer client.Quit()

		if err := client.Auth(auth); err != nil {
			return err
		}
		if err := client.Mail(s.fromAddress); err != nil {
			return err
		}
		if err := client.Rcpt(strings.TrimSpace(toAddress)); err != nil {
			return err
		}
		writer, err := client.Data()
		if err != nil {
			return err
		}
		if _, err := writer.Write([]byte(message)); err != nil {
			_ = writer.Close()
			return err
		}
		return writer.Close()
	}

	client, err := smtp.Dial(addr)
	if err != nil {
		return err
	}
	defer client.Quit()

	if ok, _ := client.Extension("STARTTLS"); ok {
		if err := client.StartTLS(&tls.Config{ServerName: s.host}); err != nil {
			return err
		}
	}
	if err := client.Auth(auth); err != nil {
		return err
	}
	if err := client.Mail(s.fromAddress); err != nil {
		return err
	}
	if err := client.Rcpt(strings.TrimSpace(toAddress)); err != nil {
		return err
	}
	writer, err := client.Data()
	if err != nil {
		return err
	}
	if _, err := writer.Write([]byte(message)); err != nil {
		_ = writer.Close()
		return err
	}
	return writer.Close()
}

func inferSMTPHost(identity string) string {
	parts := strings.Split(strings.TrimSpace(identity), "@")
	if len(parts) != 2 || strings.TrimSpace(parts[1]) == "" {
		return ""
	}
	return "smtp." + strings.TrimSpace(parts[1])
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func buildVerificationCodeHTML(brandName, code string) string {
	subtitle := "• Creative Image Workspace •"
	tagline := "多人共享的 AI 图像工作区"
	codeCells := buildCodeCells(code)

	return fmt.Sprintf(`<!DOCTYPE html PUBLIC "-//W3C//DTD XHTML 1.0 Transitional//EN" "http://www.w3.org/TR/xhtml1/DTD/xhtml1-transitional.dtd">
<html xmlns="http://www.w3.org/1999/xhtml">
<head>
    <meta http-equiv="Content-Type" content="text/html; charset=UTF-8" />
    <title>%s 验证码</title>
    <meta name="viewport" content="width=device-width, initial-scale=1.0"/>
</head>
<body style="margin: 0; padding: 0; background-color: #f4f0ff; font-family: 'Microsoft YaHei', Helvetica, Arial, sans-serif;">
    <table border="0" cellpadding="0" cellspacing="0" width="100%%" style="background-color: #f4f0ff; padding: 40px 0;">
        <tr>
            <td align="center">
                <table border="0" cellpadding="0" cellspacing="0" width="700" style="background-color: #ffffff; border-radius: 15px; overflow: hidden; box-shadow: 0 10px 30px rgba(165,143,255,0.1);">
                    <tr>
                        <td style="padding: 30px 40px;">
                            <table border="0" cellpadding="0" cellspacing="0" width="100%%">
                                <tr>
                                    <td width="50" align="left">
                                        <img src="cid:logo_icon" width="40" height="40" alt="Logo" style="display: block; border: 0;" />
                                    </td>
                                    <td align="left">
                                        <div style="font-size: 20px; font-weight: bold; color: #5a4a9c; margin: 0;">%s</div>
                                        <div style="font-size: 11px; color: #a58fff; letter-spacing: 1px;">%s</div>
                                    </td>
                                    <td align="right" style="font-size: 12px; color: #999;">%s</td>
                                </tr>
                            </table>
                        </td>
                    </tr>
                    <tr>
                        <td style="padding: 0 40px;">
                            <div style="border-top: 1px dashed #ede9ff; font-size: 1px; line-height: 1px;">&nbsp;</div>
                        </td>
                    </tr>
                    <tr>
                        <td style="padding: 40px 0 0 40px;">
                            <table border="0" cellpadding="0" cellspacing="0" width="100%%">
                                <tr>
                                    <td width="380" valign="top" style="padding-bottom: 40px;">
                                        <div style="font-size: 22px; font-weight: bold; color: #333; margin-bottom: 20px;">✦ 亲爱的用户：</div>
                                        <p style="font-size: 14px; color: #666; line-height: 1.6; margin-bottom: 30px;">
                                            欢迎使用 %s。您的邮箱验证码如下，请在 <span style="color: #ff75a4; font-weight: bold;">10 分钟</span>内完成验证。
                                        </p>
                                        <div style="font-size: 14px; font-weight: bold; color: #333; margin-bottom: 15px;">验证码 ✦</div>
                                        <table border="0" cellpadding="0" cellspacing="0" style="margin-bottom: 20px;">
                                            <tr>
                                                %s
                                            </tr>
                                        </table>
                                        <p style="font-size: 12px; color: #b0b0b0; margin-bottom: 40px;">⚠ 验证码将在 10 分钟后失效，请尽快使用。</p>
                                        <table border="0" cellpadding="0" cellspacing="0" width="340" style="background-color: #f9f8ff; border: 1px solid #e7e1ff; border-radius: 10px;">
                                            <tr>
                                                <td style="padding: 15px;">
                                                    <table border="0" cellpadding="0" cellspacing="0" width="100%%">
                                                        <tr>
                                                            <td width="60" valign="top">
                                                                <img src="cid:chibi" width="50" height="50" alt="tip" style="display: block; border: 0;" />
                                                            </td>
                                                            <td valign="top">
                                                                <div style="font-size: 14px; color: #a58fff; font-weight: bold; margin-bottom: 5px;">这不是你本人操作？</div>
                                                                <div style="font-size: 12px; color: #aaa; line-height: 1.4;">如果这不是您本人的操作，请忽略此邮件，您的账号安全不会受到影响。</div>
                                                            </td>
                                                        </tr>
                                                    </table>
                                                </td>
                                            </tr>
                                        </table>
                                    </td>
                                    <td width="280" valign="bottom" align="right" style="padding-right: 0;">
                                        <img src="cid:character" width="280" alt="character" style="display: block; border: 0; margin-left: auto;" />
                                    </td>
                                </tr>
                            </table>
                        </td>
                    </tr>
                    <tr>
                        <td align="left" style="background-color: #a58fff; padding: 25px 40px; color: #ffffff; font-size: 12px;">
                            此邮件由系统自动发送，请勿回复。
                        </td>
                    </tr>
                </table>
            </td>
        </tr>
    </table>
</body>
</html>`, brandName, brandName, subtitle, tagline, brandName, codeCells)
}

func buildCodeCells(code string) string {
	digits := strings.Split(strings.TrimSpace(code), "")
	cells := make([]string, 0, len(digits)*2)
	for index, digit := range digits {
		cells = append(cells, fmt.Sprintf(`<td width="45" height="55" align="center" style="background-color: #f9f8ff; border: 1px solid #e7e1ff; border-radius: 6px; font-size: 24px; font-weight: bold; color: #5a4a9c;">%s</td>`, digit))
		if index < len(digits)-1 {
			cells = append(cells, `<td width="10"></td>`)
		}
	}
	return strings.Join(cells, "")
}

func buildInlineHTMLMessage(html string) (string, string, error) {
	const boundary = "cheilins-studio-related"
	var builder strings.Builder

	parts := []struct {
		contentID   string
		contentType string
		filename    string
		data        []byte
	}{
		{contentID: "logo_icon", contentType: "image/png", filename: "logo_icon.png", data: logoIconPNG},
		{contentID: "chibi", contentType: "image/png", filename: "chibi.png", data: chibiPNG},
		{contentID: "character", contentType: "image/png", filename: "character.png", data: characterPNG},
	}

	builder.WriteString("--" + boundary + "\r\n")
	builder.WriteString("Content-Type: text/html; charset=UTF-8\r\n")
	builder.WriteString("Content-Transfer-Encoding: 8bit\r\n\r\n")
	builder.WriteString(html + "\r\n")

	for _, part := range parts {
		encoded, err := wrapBase64(part.data)
		if err != nil {
			return "", "", err
		}
		builder.WriteString("--" + boundary + "\r\n")
		builder.WriteString("Content-Type: " + part.contentType + "; name=\"" + part.filename + "\"\r\n")
		builder.WriteString("Content-Transfer-Encoding: base64\r\n")
		builder.WriteString("Content-ID: <" + part.contentID + ">\r\n")
		builder.WriteString("Content-Disposition: inline; filename=\"" + part.filename + "\"\r\n\r\n")
		builder.WriteString(encoded + "\r\n")
	}

	builder.WriteString("--" + boundary + "--")
	return builder.String(), "multipart/related; boundary=" + boundary, nil
}

func wrapBase64(data []byte) (string, error) {
	if len(data) == 0 {
		return "", fmt.Errorf("inline mail asset is empty")
	}

	var encoded bytes.Buffer
	encoder := base64.NewEncoder(base64.StdEncoding, &encoded)
	if _, err := encoder.Write(data); err != nil {
		return "", err
	}
	if err := encoder.Close(); err != nil {
		return "", err
	}

	raw := encoded.String()
	lines := make([]string, 0, (len(raw)/76)+1)
	for len(raw) > 76 {
		lines = append(lines, raw[:76])
		raw = raw[76:]
	}
	if raw != "" {
		lines = append(lines, raw)
	}
	return strings.Join(lines, "\r\n"), nil
}
