package utils

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/resendlabs/resend-go"
)

func SendOrderEmail(
	shopName string,
	recipients []string,
	fullName, phoneNumber, state, city, productName, variant, shippingMethod string,
	quantity uint,
	price, shippingPrice, totalPrice float64,
) error {
	if len(recipients) == 0 {
		return nil
	}

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
	<title>Nouvelle Commande - %s</title>
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
		<div class="header">Nouvelle Commande Reçue (%s)</div>
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
		Cet email a été envoyé automatiquement par %s.
		</div>
	</div>
	</body>
	</html>`,
		shopName, shopName,
		fullName, phoneNumber, state, city, productName, variant, quantity, price, shippingMethod, shippingPrice, totalPrice,
		shopName)

	// Build the email request
	params := &resend.SendEmailRequest{
		From:    "DZ Bazar <contact@lkparfumo.com>",
		To:      recipients,
		Subject: fmt.Sprintf("Nouvelle Commande – %s – %s – %s – %d", shopName, fullName, phoneNumber, time.Now().Unix()),
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

func SendLowStockEmail(productName, variant string, quantity int) error {
	client := resend.NewClient(os.Getenv("RESEND_API_KEY"))
	if client == nil {
		return fmt.Errorf("failed to initialize Resend client")
	}

	htmlContent := fmt.Sprintf(`
	<!DOCTYPE html>
	<html lang="fr">
	<head>
	<meta charset="UTF-8">
	<title>Alerte Stock Faible - LK Parfumo</title>
	<style>
		body { font-family: Arial, sans-serif; background-color: #f4f4f4; padding: 20px; }
		.container { max-width: 600px; margin: auto; background: #ffffff; padding: 20px; border-radius: 8px; }
		.header { background: #dc3545; color: white; text-align: center; padding: 15px; font-size: 22px; font-weight: bold; border-radius: 8px 8px 0 0; }
		.content { padding: 20px; font-size: 16px; line-height: 1.5; color: #333; }
		.content p { margin: 8px 0; }
		.footer { text-align: center; font-size: 14px; color: #666; margin-top: 20px; }
	</style>
	</head>
	<body>
	<div class="container">
		<div class="header">Alerte Stock Faible</div>
		<div class="content">
			<p><strong>Produit :</strong> %s</p>
			<p><strong>Variant :</strong> %s</p>
			<p><strong>Quantité restante :</strong> %d</p>
			<p>Le stock de ce produit est désormais inférieur à 10.</p>
			<p>Veuillez réapprovisionner ce produit dès que possible.</p>
		</div>
		<div class="footer">
			Cet email a été envoyé automatiquement par LK Parfumo.
		</div>
	</div>
	</body>
	</html>`,
		productName, variant, quantity)

	params := &resend.SendEmailRequest{
		From:    "LK Parfumo <contact@lkparfumo.com>",
		To:      []string{"chtiwaa@gmail.com"},
		Cc:      []string{"lakhalzineddine12@gmail.com"},
		Subject: fmt.Sprintf("Alerte stock faible – %s – %s", productName, variant),
		Html:    htmlContent,
		Headers: map[string]string{
			"Message-ID": fmt.Sprintf("<%d-%s@lkparfumo>", time.Now().UnixNano(), uuid.New().String()),
		},
	}

	resp, err := client.Emails.Send(params)
	if err != nil {
		log.Printf("Resend error: %v\n", err)
		return fmt.Errorf("resend error: %v", err)
	}

	if resp.Id == "" {
		log.Printf("Resend response error: empty response ID")
		return fmt.Errorf("resend error: failed to send email")
	}

	log.Printf("Low stock email sent successfully: ID %s\n", resp.Id)
	return nil
}

func SendPasswordResetEmail(email, otp string) error {
	client := resend.NewClient(os.Getenv("RESEND_API_KEY"))
	if client == nil {
		return fmt.Errorf("failed to initialize Resend client")
	}

	htmlContent := fmt.Sprintf(`
	<!DOCTYPE html>
	<html lang="fr">
	<head>
	<meta charset="UTF-8">
	<title>Réinitialisation du mot de passe - LK Parfumo</title>
	<style>
		body { font-family: Arial, sans-serif; background-color: #f4f4f4; padding: 20px; }
		.container { max-width: 600px; margin: auto; background: #ffffff; padding: 20px; border-radius: 8px; text-align: center; }
		.header { background: #000000; color: white; padding: 15px; font-size: 22px; font-weight: bold; border-radius: 8px 8px 0 0; }
		.content { padding: 30px 20px; font-size: 16px; color: #333; }
		.otp { font-size: 32px; font-weight: bold; letter-spacing: 8px; margin: 20px 0; color: #000000; }
		.note { font-size: 14px; color: #666; margin-top: 20px; }
	</style>
	</head>
	<body>
		<div class="container">
			<div class="header">Réinitialisation du mot de passe</div>
			<div class="content">
				<p>Voici votre code de réinitialisation :</p>
				<div class="otp">%s</div>
				<p>Ce code expire dans 15 minutes.</p>
				<p class="note">Si vous n'avez pas demandé cette réinitialisation, ignorez cet email.</p>
			</div>
		</div>
	</body>
	</html>`, otp)

	params := &resend.SendEmailRequest{
		From:    "LK Parfumo <contact@lkparfumo.com>",
		To:      []string{email},
		Subject: "Réinitialisation de votre mot de passe",
		Html:    htmlContent,
		Headers: map[string]string{
			"Message-ID": fmt.Sprintf("<%d-%s@lkparfumo>", time.Now().UnixNano(), uuid.New().String()),
		},
	}

	resp, err := client.Emails.Send(params)
	if err != nil {
		log.Printf("Resend error: %v\n", err)
		return fmt.Errorf("resend error: %v", err)
	}

	if resp.Id == "" {
		return fmt.Errorf("resend error: failed to send email")
	}

	log.Printf("Password reset email sent successfully: ID %s\n", resp.Id)
	return nil
}

func SendOTPEmail(email, otp string) error {
	client := resend.NewClient(os.Getenv("RESEND_API_KEY"))
	if client == nil {
		return fmt.Errorf("failed to initialize Resend client")
	}

	htmlContent := fmt.Sprintf(`
	<!DOCTYPE html>
	<html lang="fr">
	<head>
	<meta charset="UTF-8">
	<title>Code de vérification - LK Parfumo</title>
	<style>
		body {
			font-family: Arial, sans-serif;
			background-color: #f4f4f4;
			padding: 20px;
		}
		.container {
			max-width: 600px;
			margin: auto;
			background: #ffffff;
			padding: 20px;
			border-radius: 8px;
			text-align: center;
		}
		.header {
			background: #000000;
			color: white;
			padding: 15px;
			font-size: 22px;
			font-weight: bold;
			border-radius: 8px 8px 0 0;
		}
		.content {
			padding: 30px 20px;
			font-size: 16px;
			color: #333;
		}
		.otp {
			font-size: 32px;
			font-weight: bold;
			letter-spacing: 8px;
			margin: 20px 0;
			color: #000000;
		}
		.note {
			font-size: 14px;
			color: #666;
			margin-top: 20px;
		}
	</style>
	</head>
	<body>
		<div class="container">
			<div class="header">Vérification de votre email</div>
			<div class="content">
				<p>Voici votre code de vérification :</p>
				<div class="otp">%s</div>
				<p>Ce code expire dans 15 minutes.</p>
				<p class="note">Si vous n'avez pas demandé ce code, ignorez cet email.</p>
			</div>
		</div>
	</body>
	</html>`, otp)

	params := &resend.SendEmailRequest{
		From:    "DZ BAZAR <contact@lkparfumo.com>",
		To:      []string{email},
		Subject: "Votre code de vérification",
		Html:    htmlContent,
		Headers: map[string]string{
			"Message-ID": fmt.Sprintf("<%d-%s@lkparfumo>", time.Now().UnixNano(), uuid.New().String()),
		},
	}

	resp, err := client.Emails.Send(params)
	if err != nil {
		log.Printf("Resend error: %v\n", err)
		return fmt.Errorf("resend error: %v", err)
	}

	if resp.Id == "" {
		log.Printf("Resend response error: empty response ID")
		return fmt.Errorf("resend error: failed to send email")
	}

	log.Printf("OTP email sent successfully: ID %s\n", resp.Id)
	return nil
}

func SendPlanExpiryEmail(email, shopName, planName string, expiresAt time.Time) error {
	client := resend.NewClient(os.Getenv("RESEND_API_KEY"))
	if client == nil {
		return fmt.Errorf("failed to initialize Resend client")
	}

	htmlContent := fmt.Sprintf(`
	<!DOCTYPE html>
	<html lang="fr">
	<head>
	<meta charset="UTF-8">
	<title>Expiration de votre abonnement - DZ Bazar</title>
	<style>
		body { font-family: Arial, sans-serif; background-color: #f4f4f4; padding: 20px; }
		.container { max-width: 600px; margin: auto; background: #ffffff; padding: 20px; border-radius: 8px; text-align: center; }
		.header { background: #000000; color: white; padding: 15px; font-size: 22px; font-weight: bold; border-radius: 8px 8px 0 0; }
		.content { padding: 30px 20px; font-size: 16px; color: #333; text-align: left; }
		.content p { margin: 8px 0; }
		.footer { text-align: center; font-size: 14px; color: #666; margin-top: 20px; }
	</style>
	</head>
	<body>
		<div class="container">
			<div class="header">Expiration de votre abonnement</div>
			<div class="content">
				<p>Bonjour,</p>
				<p>Votre abonnement <strong>%s</strong> pour la boutique <strong>%s</strong> expire le <strong>%s</strong>, soit dans 3 jours.</p>
				<p>Contactez-nous pour renouveler votre abonnement.</p>
			</div>
			<div class="footer">
				Cet email a été envoyé automatiquement par LK Parfumo.
			</div>
		</div>
	</body>
	</html>`, planName, shopName, expiresAt.Format("02/01/2006"))

	params := &resend.SendEmailRequest{
		From:    "LK Parfumo <contact@lkparfumo.com>",
		To:      []string{email},
		Subject: fmt.Sprintf("Votre abonnement %s expire bientôt – %s", planName, shopName),
		Html:    htmlContent,
		Headers: map[string]string{
			"Message-ID": fmt.Sprintf("<%d-%s@lkparfumo>", time.Now().UnixNano(), uuid.New().String()),
		},
	}

	resp, err := client.Emails.Send(params)
	if err != nil {
		log.Printf("Resend error: %v\n", err)
		return fmt.Errorf("resend error: %v", err)
	}

	if resp.Id == "" {
		log.Printf("Resend response error: empty response ID")
		return fmt.Errorf("resend error: failed to send email")
	}

	log.Printf("Plan expiry email sent successfully: ID %s\n", resp.Id)
	return nil
}
