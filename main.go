package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/caarlos0/env/v6"
	"github.com/forsam-education/hermes/storageconnector"
	"gopkg.in/gomail.v2"
	htemplate "html/template"
	"os"
	ttemplate "text/template"
)

type config struct {
	Bucket       string `env:"TEMPLATE_BUCKET"`
	SMTPHost     string `env:"SMTP_HOST"`
	SMTPPort     int    `env:"SMTP_PORT" envDefault:"465"`
	SMTPUserName string `env:"SMTP_USER"`
	SMTPPassword string `env:"SMTP_PASS"`
}

type mailMessage struct {
	FromName        string                 `json:"from_name"`
	FromAddress     string                 `json:"from_address"`
	ToAddress       string                 `json:"to_address"`
	ReplyToAddress  string                 `json:"reply_to"`
	Template        string                 `json:"template_name"`
	Subject         string                 `json:"subject"`
	CC              []string               `json:"cc,omitempty"`
	BCC             []string               `json:"bcc,omitempty"`
	TemplateContext map[string]interface{} `json:"template_context"`
}

func exitErrorf(msg string, args ...interface{}) {
	_, _ = fmt.Fprintf(os.Stderr, msg+"\n", args...)
	os.Exit(1)
}

func buildMailContent(storageConnector storageconnector.StorageConnector, mailMsg *mailMessage) *gomail.Message {
	message := gomail.NewMessage()

	htmlTmpl, _ := htemplate.New("htmlTemplate").Parse(storageConnector.GetTemplateContent(fmt.Sprintf("%s.html.template", mailMsg.Template)))
	txtTmpl, _ := ttemplate.New("textTemplate").Parse(storageConnector.GetTemplateContent(fmt.Sprintf("%s.txt.template", mailMsg.Template)))

	var htmlTmplBuffer bytes.Buffer
	err := htmlTmpl.Execute(&htmlTmplBuffer, mailMsg.TemplateContext)
	if err != nil {
		exitErrorf("Unable to execute HTML template: %+v\n", err)
	}

	var txtTmplBuffer bytes.Buffer
	err = txtTmpl.Execute(&txtTmplBuffer, mailMsg.TemplateContext)
	if err != nil {
		exitErrorf("Unable to execute TXT template: %+v\n", err)
	}

	ccAddresses := make([]string, len(mailMsg.CC))
	for i, ccRecipient := range mailMsg.CC {
		ccAddresses[i] = message.FormatAddress(ccRecipient, "")
	}

	bccAddresses := make([]string, len(mailMsg.BCC))
	for i, bccRecipient := range mailMsg.BCC {
		bccAddresses[i] = message.FormatAddress(bccRecipient, "")
	}

	message.SetBody("text/plain", txtTmplBuffer.String())
	message.AddAlternative("text/html", htmlTmplBuffer.String())
	message.SetAddressHeader("From", mailMsg.FromAddress, mailMsg.FromName)
	message.SetHeader("To", mailMsg.ToAddress)
	message.SetHeader("Subject", mailMsg.Subject)
	message.SetHeader("Cc", ccAddresses...)
	message.SetHeader("Bcc", bccAddresses...)
	message.SetHeader("Reply-To", mailMsg.ReplyToAddress)

	return message
}

// HandleRequest is the main handler function used by the lambda runtime for the incomming event.
func HandleRequest(_ context.Context, event events.SQSEvent) error {
	cfg := config{}
	if err := env.Parse(&cfg); err != nil {
		exitErrorf("Unable to parse configuration: %+v\n", err)
	}

	smtpTransport := gomail.NewDialer(cfg.SMTPHost, cfg.SMTPPort, cfg.SMTPUserName, cfg.SMTPPassword)

	for _, message := range event.Records {
		var mailMsg mailMessage

		err := json.Unmarshal([]byte(message.Body), &mailMsg)
		if err != nil {
			exitErrorf("Unable to unmarshal JSON for reason: %+v\nBody: %s", err, message.Body)
		}

		storageConnector := storageconnector.NewS3(cfg.Bucket)

		mail := buildMailContent(storageConnector, &mailMsg)

		if err := smtpTransport.DialAndSend(mail); err != nil {
			exitErrorf("Unable to send mail: %+v\n", err)
		}
	}

	return nil
}

func main() {
	lambda.Start(HandleRequest)
}
