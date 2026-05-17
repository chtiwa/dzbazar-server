1. Shops

- the shop can only have one owner
- the shop can have multiple users (Memberships)
- the user can belong to multiple shops
- when the usr logs in, we check if the owner_id = user.ID, if they have multiple shops, they can choose their store, when they click a shop, the frontend saves that specific shopID and appends it as a header, X-Shop-ID: <uuid>
