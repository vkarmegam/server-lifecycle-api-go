-- migrations/schema.sql

CREATE TABLE servers (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(255) NOT NULL,
    region VARCHAR(100) NOT NULL,
    status VARCHAR(15) NOT NULL DEFAULT 'provisioning',
    address VARCHAR(15) NOT NULL DEFAULT 'NOT SERVED',
    type VARCHAR(10) NOT NULL,
    provisioned_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_status_update TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    uptime_seconds BIGINT NOT NULL DEFAULT 0,
    hourly_cost DOUBLE PRECISION NOT NULL,
    lifecycle_logs JSONB NOT NULL DEFAULT '[]'::jsonb, 
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE ip_addresses (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    address VARCHAR(15) NOT NULL UNIQUE, 
    is_allocated BOOLEAN NOT NULL DEFAULT FALSE,
    server_id UUID UNIQUE REFERENCES servers(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
