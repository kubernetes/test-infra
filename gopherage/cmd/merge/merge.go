package merge

import (
	"github.com/spf13/cobra"
	"golang.org/x/tools/cover"
	"log"
	"k8s.io/test-infra/gopherage/pkg/cov"
	"k8s.io/test-infra/gopherage/pkg/util"
)

type flags struct {
	OutputFile string
}

func MakeCommand() *cobra.Command {
	flags := &flags{}
	cmd := &cobra.Command{
		Use:   "merge [files...]",
		Short: "Merge multiple coherent Go coverage files into a single file.",
		Long: `merge will merge multiple Go coverage files into a single coverage file.
merge requires that the files are 'coherent', meaning that if they both contain references to the
same paths, then the contents of those source files were identical for the binary that generated
each file.

Merging a single file is a no-op, but is supported for convenience when shell scripting.`,
		Run: func(cmd *cobra.Command, args []string) {
			run(flags, cmd, args)
		},
	}
	cmd.Flags().StringVar(&flags.OutputFile, "o", "-", "output file")
	return cmd
}

func run(flags *flags, cmd *cobra.Command, args []string) {
	if len(args) == 0 {
		log.Fatalf("expected at least one file")
	}

	profiles := make([][]*cover.Profile, len(args))
	for _, path := range args {
		profile, err := cover.ParseProfiles(path)
		if err != nil {
			log.Fatalf("failed to open %s: %v", path, err)
		}
		profiles = append(profiles, profile)
	}

	merged, err := cov.MergeMultipleProfiles(profiles)
	if err != nil {
		log.Fatalf("failed to merge files: %v", err)
	}

	if err := util.DumpProfile(flags.OutputFile, merged); err != nil {
		log.Fatalln(err)
	}
}
