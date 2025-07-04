package utils

import (
	"fmt"
	"log"
	"os"

	"github.com/sendgrid/sendgrid-go"
	"github.com/sendgrid/sendgrid-go/helpers/mail"
)

func SendEmail(
	fullName, phoneNumber, state, city, productName, variant, shippingMethod string,
	quantity uint,
	price, shippingPrice, totalPrice float64,
) error {

	from := mail.NewEmail("LK Parfumo", "support@lkparfumo.com")
	subject := "Nouvelle Commande (LK Parfumo)"

	// Recipients
	to1 := mail.NewEmail("Sifeddine Djeddid", "chtiwaa@gmail.com")
	// to2 := mail.NewEmail("Admin", "lakhalzineddine12@gmail.com")

	// HTML body
	htmlContent := fmt.Sprintf(`
	<!DOCTYPE html>
	<html lang="fr">
	<head>
	<meta charset="UTF-8">
	<title>Nouvelle Commande - LK Parfumo</title>
	<style>
		body { font-family: Arial, sans-serif; background-color: #f4f4f4; padding: 20px; }
		.container { max-width: 600px; margin: auto; background: #ffffff; padding: 20px; border-radius: 8px; }
		.header { background: #007bff; color: white; text-align: center; padding: 15px; font-size: 22px; font-weight: bold; border-radius: 8px 8px 0 0; }
		.order-details { padding: 20px; font-size: 16px; line-height: 1.5; color: #333; }
		.order-details p { margin: 8px 0; }
		.footer { text-align: center; font-size: 14px; color: #666; margin-top: 20px; }
	</style>
	</head>
	<body>
	<div class="container">
		<div class="header">Nouvelle Commande Reçue (LK Parfumo)</div>
		<div class="order-details">
		<p><strong>Nom:</strong> %s</p>
		<p><strong>Téléphone:</strong> %s</p>
		<p><strong>Wilaya:</strong> %s</p>
		<p><strong>Commune:</strong> %s</p>
		<p><strong>Produit:</strong> %s</p>
		<p><strong>Variant:</strong> %s</p>
		<p><strong>Quantité:</strong> %d</p>
		<p><strong>Prix unitaire:</strong> %.2f DA</p>
		<p><strong>Méthode de livraison:</strong> %s</p>
		<p><strong>Frais de livraison:</strong> %.2f DA</p>
		<p><strong>Prix total:</strong> %.2f DA</p>
		</div>
		<div class="footer">
		Cet email a été envoyé automatiquement par LK Parfumo.
		</div>
	</div>
	</body>
	</html>`,
		fullName, phoneNumber, state, city, productName, variant, quantity, price, shippingMethod, shippingPrice, totalPrice)

	// Build the message
	message := mail.NewV3Mail()
	message.SetFrom(from)
	message.Subject = subject

	// Personalization: add both recipients
	personalization := mail.NewPersonalization()
	// personalization.AddTos(to1, to2)
	personalization.AddTos(to1)
	message.AddPersonalizations(personalization)

	// Add HTML content
	content := mail.NewContent("text/html", htmlContent)
	message.AddContent(content)

	// Send the email
	client := sendgrid.NewSendClient(os.Getenv("SENDGRID_API_KEY"))
	response, err := client.Send(message)
	if err != nil {
		log.Printf("SendGrid error: %v\n", err)
		return err
	}

	if response.StatusCode >= 400 {
		log.Printf("SendGrid response error: Status %d, Body: %s\n", response.StatusCode, response.Body)
		return fmt.Errorf("sendgrid error: status %d", response.StatusCode)
	}

	log.Printf("Email sent successfully: Status %d\n", response.StatusCode)
	return nil
}
