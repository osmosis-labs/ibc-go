package keeper

import (
	"reflect"

	errorsmod "cosmossdk.io/errors"
	"github.com/cosmos/cosmos-sdk/telemetry"
	sdk "github.com/cosmos/cosmos-sdk/types"
	capabilitytypes "github.com/cosmos/cosmos-sdk/x/capability/types"

	clienttypes "github.com/cosmos/ibc-go/v7/modules/core/02-client/types"
	connectiontypes "github.com/cosmos/ibc-go/v7/modules/core/03-connection/types"
	"github.com/cosmos/ibc-go/v7/modules/core/04-channel/types"
	host "github.com/cosmos/ibc-go/v7/modules/core/24-host"
)

// ChanUpgradeInit is called by a module to initiate a channel upgrade handshake with
// a module on another chain.
func (k Keeper) ChanUpgradeInit(
	ctx sdk.Context,
	portID string,
	channelID string,
	chanCap *capabilitytypes.Capability,
	proposedUpgradeChannel types.Channel,
	counterpartyTimeoutHeight clienttypes.Height,
	counterpartyTimeoutTimestamp uint64,
) (upgradeSequence uint64, previousVersion string, err error) {
	channel, found := k.GetChannel(ctx, portID, channelID)
	if !found {
		return 0, "", errorsmod.Wrapf(types.ErrChannelNotFound, "port ID (%s) channel ID (%s)", portID, channelID)
	}

	if channel.State != types.OPEN {
		return 0, "", errorsmod.Wrapf(types.ErrInvalidChannelState, "expected %s, got %s", types.OPEN, channel.State)
	}

	if !k.scopedKeeper.AuthenticateCapability(ctx, chanCap, host.ChannelCapabilityPath(portID, channelID)) {
		return 0, "", errorsmod.Wrapf(types.ErrChannelCapabilityNotFound, "caller does not own capability for channel, port ID (%s) channel ID (%s)", portID, channelID)
	}

	// set the restore channel to the current channel and reassign channel state to INITUPGRADE,
	// if the channel == proposedUpgradeChannel then fail fast as no upgradable fields have been modified.
	restoreChannel := channel
	channel.State = types.INITUPGRADE
	if reflect.DeepEqual(channel, proposedUpgradeChannel) {
		return 0, "", errorsmod.Wrap(types.ErrChannelExists, "existing channel end is identical to proposed upgrade channel end")
	}

	if !k.connectionKeeper.HasConnection(ctx, proposedUpgradeChannel.ConnectionHops[0]) {
		return 0, "", errorsmod.Wrapf(connectiontypes.ErrConnectionNotFound, "failed to retrieve connection: %s", proposedUpgradeChannel.ConnectionHops[0])
	}

	if proposedUpgradeChannel.Counterparty.PortId != channel.Counterparty.PortId ||
		proposedUpgradeChannel.Counterparty.ChannelId != channel.Counterparty.ChannelId {
		return 0, "", errorsmod.Wrap(types.ErrInvalidCounterparty, "counterparty port ID and channel ID cannot be upgraded")
	}

	if !channel.Ordering.SubsetOf(proposedUpgradeChannel.Ordering) {
		return 0, "", errorsmod.Wrap(types.ErrInvalidChannelOrdering, "channel ordering must be a subset of the new ordering")
	}

	upgradeSequence = uint64(1)
	if seq, found := k.GetUpgradeSequence(ctx, portID, channelID); found {
		upgradeSequence = seq + 1
	}

	upgradeTimeout := types.UpgradeTimeout{
		TimeoutHeight:    counterpartyTimeoutHeight,
		TimeoutTimestamp: counterpartyTimeoutTimestamp,
	}

	k.SetUpgradeRestoreChannel(ctx, portID, channelID, restoreChannel)
	k.SetUpgradeSequence(ctx, portID, channelID, upgradeSequence)
	k.SetUpgradeTimeout(ctx, portID, channelID, upgradeTimeout)

	return upgradeSequence, channel.Version, nil
}

// WriteUpgradeInitChannel writes a channel which has successfully passed the UpgradeInit handshake step.
// An event is emitted for the handshake step.
func (k Keeper) WriteUpgradeInitChannel(
	ctx sdk.Context,
	portID,
	channelID string,
	upgradeSequence uint64,
	channelUpgrade types.Channel,
) {
	defer telemetry.IncrCounter(1, "ibc", "channel", "upgrade-init")

	k.SetChannel(ctx, portID, channelID, channelUpgrade)
	k.Logger(ctx).Info("channel state updated", "port-id", portID, "channel-id", channelID, "previous-state", types.OPEN.String(), "new-state", types.INITUPGRADE.String())

	emitChannelUpgradeInitEvent(ctx, portID, channelID, upgradeSequence, channelUpgrade)
}