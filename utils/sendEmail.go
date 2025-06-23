package utils

import (
	"fmt"
	"log"
	"os"

	"github.com/sendgrid/sendgrid-go"
	"github.com/sendgrid/sendgrid-go/helpers/mail"
)

func SendEmail(fullName string, phoneNumber string, state string, stateNumber uint, city string, productName string, variant string, quantity uint, price float64, shippingMethod string, shippingPrice float64, totalPrice float64) error {
	from := mail.NewEmail("Herbs Store", "djeddid.sifeddine@gmail.com")
	subject := "Nouvelle Commande ( LK Parfumo )"
	to := mail.NewEmail("Sifeddine Djeddid", "chtiwaa@gmail.com")

	htmlContent := fmt.Sprintf(`
		<!DOCTYPE html>
		<html>
		<head>
			<title>New Order Confirmation</title>
			<style>
				body { font-family: Arial, sans-serif; background-color: #f4f4f4; padding: 20px; }
				.container { max-width: 600px; margin: auto; background: #ffffff; padding: 20px; border-radius: 8px; }
				.header { background: #007bff; color: white; text-align: center; padding: 10px; font-size: 20px; font-weight: bold; }
				.order-details { padding: 20px; font-size: 16px; }
				.footer { text-align: center; font-size: 14px; color: #666; margin-top: 20px; }
			</style>
		</head>
		<body>
			<div class="container">
				<div class="header">New Order Placed ( Herbs Store )</div>
				<div class="order-details">
					<p><strong>Nom:</strong> %s</p>
					<p><strong>Numéro:</strong> %s</p>
					<p><strong>Wilaya:</strong> %s (%d)</p>
					<p><strong>Commune:</strong> %s</p>
					<p><strong>Prodcuit:</strong> %s</p>
					<p><strong>Variant:</strong> %s</p>
					<p><strong>Quantity:</strong> %d</p>
					<p><strong>Prix:</strong> %.2f DA</p>
					<p><strong>Methode de livraison:</strong> %s</p>
					<p><strong>Prix de livraison:</strong> %.2f DA</p>
					<p><strong>Prix total:</strong> %.2f DA</p>
				</div>
			</div>
		</body>
		</html>`, fullName, phoneNumber, state, stateNumber, city, productName, variant, quantity, price, shippingMethod, shippingPrice, totalPrice)

	message := mail.NewSingleEmail(from, subject, to, "", htmlContent)
	client := sendgrid.NewSendClient(os.Getenv("SENDGRID_API_KEY"))
	_, err := client.Send(message)
	if err != nil {
		log.Println("SendGrid error:", err)
	}
	return err
}
