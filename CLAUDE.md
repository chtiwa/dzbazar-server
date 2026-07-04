# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

```bash
# Run with hot reload (development)
air

# Build
go build -o ./tmp/main.exe .

# Run directly
go run .

# Add a dependency
go get <package>
go mod tidy
```

There are no tests in this project currently.

## Architecture

Go REST API using **Gin** (HTTP router) + **GORM** (ORM) + **PostgreSQL**. Entry point is `main.go`.

### Startup sequence (`main.go` → `init()`)
1. Load `.env` via `godotenv`
2. Load static wilaya data into memory (`initializers.InitStaticData`)
3. Connect to PostgreSQL (`initializers.DB`)
4. Initialize Backblaze B2 S3-compatible client (`initializers.S3Client`)
5. Connect to Redis (`initializers.RedisClient`)
6. Run GORM `AutoMigrate` (`migrate/migrate.go`) — two-phase to handle FK ordering
7. Register all routes and start the server on the port from `$PORT` (default `:8080`)

### Request lifecycle
- All shop-scoped routes follow the pattern `/api/v1/shops/:shopId/<resource>`
- `middleware.RequireAuthentication` — validates JWT from `AccessToken` cookie, auto-rotates using `RefreshToken` cookie if expired, sets `user` and `role` in gin context
- `middleware.RequireShopAccess("Owner")` — checks `X-Shop-ID` header against `shop_members` table, enforces role
- `middleware.RequireRoles(...)` — checks the `role` field on the JWT claims

### Multi-tenancy model
A `User` can belong to multiple `Shop`s via `ShopMember` (with a `role`: `owner` | `moderator` | `confirmation`). The active shop is communicated on every request via the `X-Shop-ID` header set by the frontend after login.

### Key domain relationships
- `Shop` → `Product` → `Variant` → `VariantItem`; `Product` → `ProductVariantCombination` (flattened SKU rows linking back to `VariantItem` via `Option1ID/2ID/3ID`)
- `Order` → `OrderItem` → `ProductVariantCombination` (FK is `OnDelete:RESTRICT` — never delete a combination that has orders)
- `Order` → `Client` (upserted by phone number per shop on order creation)
- `Shop` → `DeliveryCompany` (per-shop credentials) → `AvailableDeliveryCompany` (global admin-managed list with name, URL, image)
- `Shop` → `DeliveryRate` (one row per wilaya, seeded at shop creation from static wilaya config)
- `Shop` → `Pixel` (Facebook/TikTok conversion tracking pixels)

### Product variant update constraint
When updating variants/combinations on a product, the order matters:
1. Null out `option1_id/2/3` on existing combinations first (FK to `variant_items`)
2. Delete variants (cascades to `variant_items`)
3. Recreate variants and items
4. Upsert combinations by SKU — update existing, create new, retire removed ones (set `quantity=0` if referenced by orders, hard-delete otherwise)

### Image storage
All images upload to Backblaze B2 via the AWS S3-compatible SDK. The upload helper pattern (content-type sniff → seek reset → `PutObject`) is consistent across shops, products, and delivery companies. Public URL is constructed from `B2_PUBLIC_BASE_URL` env var if set, otherwise falls back to the standard B2 URL format.

### Async post-order tasks
After a successful order creation, a goroutine fires email notification (Resend), Facebook CAPI purchase event, and a WebSocket broadcast to connected dashboard clients. It has a `recover()` guard and re-fetches the full order from DB before processing.

### Environment variables required
`DB_URI`, `JWT_SECRET`, `APP_ENV`, `B2_BUCKET_NAME`, `B2_REGION`, `B2_PUBLIC_BASE_URL`, `B2_KEY_ID`, `B2_APP_KEY`, `B2_ENDPOINT`, `REDIS_URL`, `RESEND_API_KEY`, `FACEBOOK_TEST_CODE` (dev only)

Carrier credentials (Osen Express, Leopard Express, ZR Express) are per-shop `DeliveryCompany` rows, not env vars.
