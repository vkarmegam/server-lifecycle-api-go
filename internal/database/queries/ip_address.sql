-- sql/ip_address.sql

-- name: CreateIPAddress :one
INSERT INTO ip_addresses (address)
VALUES ($1)
RETURNING *;

-- name: GetAvailableIPForAllocation :one
SELECT * FROM ip_addresses
WHERE is_allocated = FALSE AND server_id IS NULL
ORDER BY created_at ASC
FOR UPDATE SKIP LOCKED
LIMIT 1;

-- name: AllocateIPAddress :one
UPDATE ip_addresses
SET is_allocated = TRUE, server_id = $1, updated_at = NOW()
WHERE id = $2 AND is_allocated = FALSE AND server_id IS NULL
RETURNING *;

-- name: DeallocateIPAddress :one
UPDATE ip_addresses
SET is_allocated = FALSE, server_id = NULL, updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: GetIPAddressByServerID :one
SELECT * FROM ip_addresses WHERE server_id = $1;

-- name: TruncateIPAddresses :exec
TRUNCATE ip_addresses RESTART IDENTITY CASCADE;