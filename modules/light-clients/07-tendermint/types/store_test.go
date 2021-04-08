package types_test

import (
	clienttypes "github.com/cosmos/ibc-go/modules/core/02-client/types"
	channeltypes "github.com/cosmos/ibc-go/modules/core/04-channel/types"
	host "github.com/cosmos/ibc-go/modules/core/24-host"
	"github.com/cosmos/ibc-go/modules/core/exported"
	solomachinetypes "github.com/cosmos/ibc-go/modules/light-clients/06-solomachine/types"
	"github.com/cosmos/ibc-go/modules/light-clients/07-tendermint/types"
	ibctesting "github.com/cosmos/ibc-go/testing"
)

func (suite *TendermintTestSuite) TestGetConsensusState() {
	var (
		height  exported.Height
		clientA string
	)

	testCases := []struct {
		name     string
		malleate func()
		expPass  bool
	}{
		{
			"success", func() {}, true,
		},
		{
			"consensus state not found", func() {
				// use height with no consensus state set
				height = height.(clienttypes.Height).Increment()
			}, false,
		},
		{
			"not a consensus state interface", func() {
				// marshal an empty client state and set as consensus state
				store := suite.chainA.App.IBCKeeper.ClientKeeper.ClientStore(suite.chainA.GetContext(), clientA)
				clientStateBz := suite.chainA.App.IBCKeeper.ClientKeeper.MustMarshalClientState(&types.ClientState{})
				store.Set(host.ConsensusStateKey(height), clientStateBz)
			}, false,
		},
		{
			"invalid consensus state (solomachine)", func() {
				// marshal and set solomachine consensus state
				store := suite.chainA.App.IBCKeeper.ClientKeeper.ClientStore(suite.chainA.GetContext(), clientA)
				consensusStateBz := suite.chainA.App.IBCKeeper.ClientKeeper.MustMarshalConsensusState(&solomachinetypes.ConsensusState{})
				store.Set(host.ConsensusStateKey(height), consensusStateBz)
			}, false,
		},
	}

	for _, tc := range testCases {
		tc := tc

		suite.Run(tc.name, func() {
			suite.SetupTest()

			clientA, _, _, _, _, _ = suite.coordinator.Setup(suite.chainA, suite.chainB, channeltypes.UNORDERED)
			clientState := suite.chainA.GetClientState(clientA)
			height = clientState.GetLatestHeight()

			tc.malleate() // change vars as necessary

			store := suite.chainA.App.IBCKeeper.ClientKeeper.ClientStore(suite.chainA.GetContext(), clientA)
			consensusState, err := types.GetConsensusState(store, suite.chainA.Codec, height)

			if tc.expPass {
				suite.Require().NoError(err)
				expConsensusState, found := suite.chainA.GetConsensusState(clientA, height)
				suite.Require().True(found)
				suite.Require().Equal(expConsensusState, consensusState)
			} else {
				suite.Require().Error(err)
				suite.Require().Nil(consensusState)
			}
		})
	}
}

func (suite *TendermintTestSuite) TestGetProcessedMetadata() {
	// Verify ProcessedTime on CreateClient
	// coordinator increments time and height before creating client
	expectedTime := suite.chainA.CurrentHeader.Time.Add(ibctesting.TimeIncrement)
	expectedHeight := clienttypes.NewHeight(0, uint64(suite.chainA.CurrentHeader.Height+1))

	clientA, err := suite.coordinator.CreateClient(suite.chainA, suite.chainB, exported.Tendermint)
	suite.Require().NoError(err)

	clientState := suite.chainA.GetClientState(clientA)
	height := clientState.GetLatestHeight()

	store := suite.chainA.App.IBCKeeper.ClientKeeper.ClientStore(suite.chainA.GetContext(), clientA)
	actualTime, ok := types.GetProcessedTime(store, height)
	suite.Require().True(ok, "could not retrieve processed time for stored consensus state")
	suite.Require().Equal(uint64(expectedTime.UnixNano()), actualTime, "retrieved processed time is not expected value")
	processedHeight, ok := types.GetProcessedHeight(store, height)
	suite.Require().True(ok, "could not retrieve processed height for stored consensus state")
	suite.Require().Equal(expectedHeight, processedHeight, "retrieved processed height is not expected value")

	// Verify ProcessedTime on UpdateClient
	// coordinator increments time and height before updating client
	expectedTime = suite.chainA.CurrentHeader.Time.Add(ibctesting.TimeIncrement)
	expectedHeight = clienttypes.NewHeight(0, uint64(suite.chainA.CurrentHeader.Height+1))

	err = suite.coordinator.UpdateClient(suite.chainA, suite.chainB, clientA, exported.Tendermint)
	suite.Require().NoError(err)

	clientState = suite.chainA.GetClientState(clientA)
	height = clientState.GetLatestHeight()

	store = suite.chainA.App.IBCKeeper.ClientKeeper.ClientStore(suite.chainA.GetContext(), clientA)
	actualTime, ok = types.GetProcessedTime(store, height)
	suite.Require().True(ok, "could not retrieve processed time for stored consensus state")
	suite.Require().Equal(uint64(expectedTime.UnixNano()), actualTime, "retrieved processed time is not expected value")
	processedHeight, ok = types.GetProcessedHeight(store, height)
	suite.Require().True(ok, "could not retrieve processed height for stored consensus state")
	suite.Require().Equal(expectedHeight, processedHeight, "retrieved processed height is not expected value")

	// try to get processed time and processed height for consensus height that doesn't exist in store
	_, ok = types.GetProcessedTime(store, clienttypes.NewHeight(1, 1))
	suite.Require().False(ok, "retrieved processed time for a non-existent consensus state")
	_, ok = types.GetProcessedHeight(store, clienttypes.NewHeight(1, 1))
	suite.Require().False(ok, "retrieved processed height for a non-existent consensus state")
}
