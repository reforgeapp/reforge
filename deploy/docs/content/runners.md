# Runners

Runners execute repository and model work outside the control plane.

## Built-in runner

Compose starts a built-in runner with the stack. It joins every organisation's
**Built-in** pool, which covers all repositories, including newly imported ones. The
pool cannot be edited or revoked; **Drain** pauses it and **Activate** resumes it.

The built-in runner runs two jobs at once (`--slots`, 1–16); each slot appears as its own
runner. Each job's sandbox may use up to 6 GiB of memory. Enrol extra runners in other
pools for more capacity, private networks or isolation from the control-plane host.

## Pools

A pool may start empty. Create a pool, then enrol a runner. Pool updates invalidate
existing enrolments when the scope changes.

## Enrolment

Enrolment produces a one-time instruction that expires. There is no permanent token in a
downloadable file. The runner registers outbound, so no inbound port is required.

## Placement

Run placement explains why a private repository or subscription agent requires a specific
pool. A private repository needs a pool bound to its approved private route.

## Drain and rotation

Draining a pool stops new claims while in-flight work finishes. Rotate a runner by
revoking it and enrolling a replacement; revocation fences any late upload or publish.

## Host prerequisites

The built-in runner runs privileged with its own cgroup namespace and needs cgroup v2.
Without it the container exits with `built-in runner needs a privileged container with
cgroup v2 delegation`; stop it with `docker compose stop runner` and enrol a runner
elsewhere.

An enrolled runner needs an OCI runtime and, for untrusted repositories, a sandbox with working
resource enforcement. On hosts without cgroup delegation the sandbox reports unavailable
and hostile repositories must not be admitted. See
[Support matrix and limitations](support-matrix.md).
