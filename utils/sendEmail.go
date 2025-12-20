package utils

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/resendlabs/resend-go"
)

func SendEmail(
	fullName, phoneNumber, state, city, productName, variant, shippingMethod string,
	quantity uint,
	price, shippingPrice, totalPrice float64,
) error {

	client := resend.NewClient(os.Getenv("RESEND_API_KEY"))
	if client == nil {
		return fmt.Errorf("failed to initialize Resend client")
	}

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

	// Build the email request
	params := &resend.SendEmailRequest{
		From:    "LK Parfumo <contact@lkparfumo.com>",
	To:      []string{"chtiwaa@gmail.com"},
		Cc:      []string{"lakhalzineddine12@gmail.com"}, // Uncomment to add CC recipient
	Subject: fmt.Sprintf("Nouvelle Commande – %s – %s – %d", fullName, phoneNumber, time.Now().Unix()),
	Html:    htmlContent,
	Headers: map[string]string{
		"Message-ID": fmt.Sprintf("<%d-%s@lkparfumo>", time.Now().UnixNano(), uuid.New().String()),
	},
}


	// Send the email
	resp, err := client.Emails.Send(params)
	if err != nil {
		log.Printf("Resend error: %v\n", err)
		return fmt.Errorf("resend error: %v", err)
	}

	// Check for non-200 status (Resend doesn't return status codes directly, but we check response ID)
	if resp.Id == "" {
		log.Printf("Resend response error: empty response ID")
		return fmt.Errorf("resend error: failed to send email")
	}

	log.Printf("Email sent successfully: ID %s\n", resp.Id)
	return nil
}
