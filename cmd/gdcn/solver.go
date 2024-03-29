package gdcn

import (
	optionsfactory "github.com/bacalhau-project/generic-dcn/pkg/options"
	"github.com/bacalhau-project/generic-dcn/pkg/solver"
	memorystore "github.com/bacalhau-project/generic-dcn/pkg/solver/store/memory"
	"github.com/bacalhau-project/generic-dcn/pkg/system"
	"github.com/bacalhau-project/generic-dcn/pkg/web3"
	"github.com/spf13/cobra"
)

func newSolverCmd() *cobra.Command {
	options := optionsfactory.NewSolverOptions()

	solverCmd := &cobra.Command{
		Use:     "solver",
		Short:   "Start the Generic Decentralized Compute Network solver service.",
		Long:    "Start the Generic Decentralized Compute Network solver service.",
		Example: "",
		RunE: func(cmd *cobra.Command, _ []string) error {
			options, err := optionsfactory.ProcessSolverOptions(options)
			if err != nil {
				return err
			}
			return runSolver(cmd, options)
		},
	}

	optionsfactory.AddSolverCliFlags(solverCmd, &options)

	return solverCmd
}

func runSolver(cmd *cobra.Command, options solver.SolverOptions) error {
	commandCtx := system.NewCommandContext(cmd)
	defer commandCtx.Cleanup()

	web3SDK, err := web3.NewContractSDK(options.Web3)
	if err != nil {
		return err
	}

	solverStore, err := memorystore.NewSolverStoreMemory()
	if err != nil {
		return err
	}

	solverService, err := solver.NewSolver(options, solverStore, web3SDK)
	if err != nil {
		return err
	}

	solverErrors := solverService.Start(commandCtx.Ctx, commandCtx.Cm)

	for {
		select {
		case err := <-solverErrors:
			commandCtx.Cleanup()
			return err
		case <-commandCtx.Ctx.Done():
			return nil
		}
	}
}
