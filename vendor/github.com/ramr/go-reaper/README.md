# go-reaper
Process (grim) reaper library for golang - this is useful for cleaning up
zombie processes inside docker containers (which do not have an init
process running as pid 1).


tl;dr
-----

       import reaper "github.com/ramr/go-reaper"

       func main() {
		//  Start background reaping of orphaned child processes.
		go reaper.Reap()

		//  Rest of your code ...
       }



How and Why
-----------
If you run a container without an init process (pid 1) which would
normally reap zombie processes, you could well end up with a lot of zombie
processes and eventually exhaust the max process limit on your system.

If you have a golang program that runs as pid 1, then this library allows
the golang program to setup a background signal handling mechanism to
handle the death of those orphaned children and not create a load of
zombies inside the pid namespace your container runs in.


Usage:
------
For basic usage, see the tl;dr section above. This should be the
most commonly used route you'd need to take.

But for those that prefer to go down "the road less traveled", you can
control whether to disable pid 1 checks and/or control the options passed to
the `wait4` (or `waitpid`) system call by passing configuration to the
reaper.


	import reaper "github.com/ramr/go-reaper"

	func main() {
		config := reaper.Config{
			Pid:              0,
			Options:          0,
			DisablePid1Check: false,
		}

		//  Start background reaping of orphaned child processes.
		go reaper.Start(config)

		//  Rest of your code ...
	}


The `Pid` and `Options` fields in the configuration are the `pid` and
`options` passed to the linux `wait4` system call.

See the man pages for the `wait4` or `waitpid` system call for details.

       https://linux.die.net/man/2/wait4
       https://linux.die.net/man/2/waitpid


