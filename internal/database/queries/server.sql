-- sql/servers.sql

-- name: CreateNewServer :one
INSERT INTO servers (name, region, status, type, address, hourly_cost)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: GetServer :one
SELECT * FROM servers WHERE id = $1;

-- name: ListServers :many
SELECT * FROM servers
WHERE status = $1
ORDER BY created_at DESC;

-- name: UpdateServerStatus :one
UPDATE servers
SET status = $1, last_status_update = NOW()
WHERE id = $2
RETURNING *;

-- name: UpdateServerUptime :one
UPDATE servers
SET uptime_seconds = $1, updated_at = NOW()
WHERE id = $2
RETURNING *;

-- name: DeleteServer :exec
DELETE FROM servers WHERE id = $1;

-- name: GetServerLifecycleLogs :one
SELECT lifecycle_logs FROM servers WHERE id = $1;

-- name: AppendServerLifecycleLog :one
UPDATE servers
SET lifecycle_logs = jsonb_build_array($1::jsonb) || lifecycle_logs
WHERE id = $2
RETURNING lifecycle_logs;

-- name: EnforceLifecycleLogsLimit :exec
UPDATE servers
SET lifecycle_logs =
    CASE
        WHEN jsonb_array_length(lifecycle_logs) > 100 
            THEN jsonb_path_query_array(lifecycle_logs, '$[0 to 14]') 
        ELSE lifecycle_logs
    END
WHERE id = $1;

-- name: TerminateAllServers :exec
UPDATE servers
SET status = 'terminated',
    last_status_update = NOW(),
    lifecycle_logs = lifecycle_logs || jsonb_build_object(
        'timestamp', NOW(),
        'event', 'System Reset: Server Terminated by System',
        'request_id', 'system-reset'
    )::jsonb
WHERE status != 'terminated';


-- name: TruncateServers :exec
TRUNCATE servers RESTART IDENTITY CASCADE;

-- name: SelectAllServers :many
SELECT * FROM servers;