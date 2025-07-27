package services

import (
	"context"
	"fmt"
	"net/netip"
	"os"
	"strings"
	"sync"

	"github.com/jackc/pgx/v5/pgtype"
	"go.uber.org/zap"

	"go-virtual-server/internal/database/sqlc"
)

// IPAllocator manages the allocation and deallocation of IP addresses.
type IPAllocator struct {
	queries *sqlc.Queries
	logger  *zap.Logger
	ipMutex sync.Mutex
}

// NewIPAllocator creates a new IPAllocator.
func NewIPAllocator(queries *sqlc.Queries, logger *zap.Logger) *IPAllocator {
	return &IPAllocator{
		queries: queries,
		logger:  logger,
	}
}

func (ipa *IPAllocator) TerminateAllServers(ctx context.Context, cidr string, exclusionList []string) error {

	ipa.ipMutex.Lock()
	defer ipa.ipMutex.Unlock()
	ipa.logger.Info("Attempting to terminate all servers")

	err := ipa.queries.TerminateAllServers(ctx)
	if err != nil {
		if strings.Contains(err.Error(), " does not exist") {
			ipa.logger.Error("SCHEMA Error:", zap.Error(err))
			os.Exit(0)
		}
		ipa.logger.Error("Failed to terminate all servers", zap.Error(err))
	}

	err = ipa.queries.TruncateIPAddresses(ctx)
	if err != nil {
		if strings.Contains(err.Error(), " does not exist") {
			ipa.logger.Error("SCHEMA Error:", zap.Error(err))
			os.Exit(0)
		}
		ipa.logger.Error("Failed to truncate IP addresses", zap.Error(err))
	}

	prefix, err := netip.ParsePrefix(cidr)
	if err != nil {
		return fmt.Errorf("invalid CIDR %q: %w", cidr, err)
	}

	exclude := make(map[netip.Addr]bool, len(exclusionList))
	for _, ex := range exclusionList {
		addr, err := netip.ParseAddr(ex)
		if err != nil {
			ipa.logger.Warn("Invalid exclusion address", zap.String("ip", ex), zap.Error(err))
			continue
		}
		exclude[addr] = true
	}
	var count int
	for addr := prefix.Addr(); prefix.Contains(addr); addr = addr.Next() {
		if exclude[addr] || addr == prefix.Addr() {
			continue
		}
		_, err := ipa.queries.CreateIPAddress(ctx, addr.String())
		if err != nil {
			continue
		}
		count++
	}

	ipa.logger.Info("IP pool pre-populated", zap.Int("added_ips", count), zap.String("cidr", cidr))
	return nil
}

// AllocateIP attempts to atomically allocate an available IP address.
func (ipa *IPAllocator) AllocateIP(ctx context.Context) (sqlc.IpAddress, error) {
	ipa.ipMutex.Lock()
	defer ipa.ipMutex.Unlock()

	availableIP, err := ipa.queries.GetAvailableIPForAllocation(ctx)
	if err != nil {
		return sqlc.IpAddress{}, err
	}

	if err != nil {
		ipa.logger.Error("IP allocation failed", zap.Error(err))
		return sqlc.IpAddress{}, err
	}

	ipa.logger.Info("Successfully selected IP for allocation", zap.String("ip_id", availableIP.ID.String()))
	return availableIP, nil
}

// AllocateIP attempts to atomically allocate an available IP address.
func (ipa *IPAllocator) saveAllocatedIP(ctx context.Context, serverID pgtype.UUID, allocatedIP pgtype.UUID) error {

	ipa.ipMutex.Lock()
	defer ipa.ipMutex.Unlock()
	ipa.logger.Info("Attempting to save allocated IP address", zap.String("allocated_ip", allocatedIP.String()), zap.String("server_id", serverID.String()))

	var allocateIP sqlc.AllocateIPAddressParams
	allocateIP.ID = allocatedIP
	allocateIP.ServerID = serverID
	// Select an available IP address, by checking ip_allocated=FALSE
	availableIP, err := ipa.queries.AllocateIPAddress(ctx, allocateIP)
	if err != nil {
		return err
	}

	if err != nil {
		ipa.logger.Error("IP allocation failed", zap.Error(err))
		return err
	}

	ipa.logger.Info("Successfully selected a IP for allocation", zap.String("ip_id", availableIP.ID.String()))
	return nil
}
