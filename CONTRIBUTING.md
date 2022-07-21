# Library files

Any library files needed by core hnvm logic should be only placed into `lib/`. The reason for this is so that the same library files can be installed in the same place on disk by our Homebrew formulas. That way, the import references are consistent for both source code/source tests and for end-user homebrew installations of hnvm.