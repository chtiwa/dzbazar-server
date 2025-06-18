package utils

import (
	"fmt"
	"log"
	"os"

	"github.com/sendgrid/sendgrid-go"
	"github.com/sendgrid/sendgrid-go/helpers/mail"
)

func SendEmail(fullName string, phoneNumber string, state string, stateNumber uint, city string, productName string, quantity uint, price float64, shippingMethod string, shippingPrice float64, totalPrice float64) error {
	from := mail.NewEmail("Shoe Shock", "djeddid.sifeddine@gmail.com")
	subject := "New Order From Herbs Store"
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
					<p><strong>Name:</strong> %s</p>
					<p><strong>Phone Number:</strong> %s</p>
					<p><strong>State:</strong> %s (%d)</p>
					<p><strong>City:</strong> %s</p>
					<p><strong>Product Name:</strong> %s</p>
					<p><strong>Quantity:</strong> %d</p>
					<p><strong>Price:</strong> %.2f DA</p>
					<p><strong>Shipping Method:</strong> %s</p>
					<p><strong>Shipping Price:</strong> %.2f DA</p>
					<p><strong>Total Price:</strong> %.2f DA</p>
				</div>
				<div class="footer">Thank you for your order!</div>
			</div>
		</body>
		</html>`, fullName, phoneNumber, state, stateNumber, city, productName, quantity, price, shippingMethod, shippingPrice, totalPrice)

	message := mail.NewSingleEmail(from, subject, to, "", htmlContent)
	client := sendgrid.NewSendClient(os.Getenv("SENDGRID_API_KEY"))
	_, err := client.Send(message)
	if err != nil {
		log.Println("SendGrid error:", err)
	}
	return err
}
