# Kubelet and systemd interaction

**Author**: Derek Carr (@derekwaynecarr)

**Status**: Proposed

## Motivation

Many Linux distributions have either adopted, or plan to adopt `systemd` as their init system.

This document describes a series of enhancements that should be made to the `kubelet` to better 
integrate with these distributions independent of their chosen container runtime.

## Scope of proposal

This proposal does not account for running the `kubelet` in a container.

## Background on systemd

### systemd units

To help understand this proposal, we first provide a brief summary of `systemd` behavior.

`systemd` manages a hierarchy of `slice`, `scope`, and `service` units.

* `service` - application on the server that is launched by `systemd`; how it should start/stop; 
when it should be started; under what circumstances it should be restarted; and any resource 
controls that should be applied to it.
* `scope` - a process or group of processes which are not launched by `systemd` (i.e. fork), like
a service, resource controls may be applied
* `slice` - organizes a hierarchy in which `scope` and `service` units are placed.  a `slice` may
contain `slice`, `scope`, or `service` units; processes are attached to `service` and `scope`
units only, not to `slices`. The hierarchy is intended to be unified, meaning a process may
only belong to a single leaf node.

## cgroup hierarchy: split versus unified hierarchies

Classical `cgroup` hierarchies were split per resource group controller, and a process could
exist in different parts of the hierarchy.

For example, a process `p1` could exist in each of the following at the same time:

* `/sys/fs/cgroup/cpu/important/`
* `/sys/fs/cgroup/memory/unimportant/`
* `/sys/fs/cgroup/cpuacct/unimportant/`

In addition, controllers for one resource group could depend on another in ways that were not
always obvious.

For example, the `cpu` controller depends on the `cpuacct` controller yet they were treated
separately.

Many found it confusing for a single process to belong to different nodes in the `cgroup` hierarchy
across controllers.

The Kernel direction for `cgroup` support is to move toward a unified `cgroup` hierarchy, where the
per-controller hierarchies are eliminated in favor of hierarchies like the following:

* `/sys/fs/cgroup/important/`
* `/sys/fs/cgroup/unimportant/`

In a unified hierarchy, a process may only belong to a single node in the `cgroup` tree.

## cgroupfs single writer

The Kernel direction for `cgroup` management is to promote a single-writer model rather than
allowing multiple processes to independently write to parts of the file-system.

In distributions that run `systemd` as their init system, the cgroup tree is managed by `systemd`
by default since it implicitly interacts with the cgroup tree when starting units.  Manual changes
made by other cgroup managers to the cgroup tree are not guaranteed to be preserved unless `systemd`
is made aware.  `systemd` can be told to ignore sections of the cgroup tree by configuring the unit
to have the `Delegate=` option.

See: http://www.freedesktop.org/software/systemd/man/systemd.resource-control.html#Delegate=

## cgroup management with systemd and container runtimes

A `slice` corresponds to an inner-node in the `cgroup` file-system hierarchy.

For example, the `system.slice` is represented as follows:

`/sys/fs/cgroup/<controller>/system.slice`

A `slice` is nested in the hierarchy by its naming convention.

For example, the `system-foo.slice` is represented as follows:

`/sys/fs/cgroup/<controller>/system.slice/system-foo.slice/`

A `service` or `scope` corresponds to leaf nodes in the `cgroup` file-system hierarchy managed by
`systemd`. Services and scopes can have child nodes managed outside of `systemd` if they have been
delegated with the `Delegate=` option.

For example, if the `docker.service` is associated with the `system.slice`, it is
represented as follows:

`/sys/fs/cgroup/<controller>/system.slice/docker.service/`

To demonstrate the use of `scope` units using the `docker` container runtime, if a
user launches a container via `docker run -m 100M busybox`, a `scope` will be created
because the process was not launched by `systemd` itself.  The `scope` is parented by
the `slice` associated with the launching daemon.

For example:

`/sys/fs/cgroup/<controller>/system.slice/docker-<container-id>.scope`

`systemd` defines a set of slices.  By default, service and scope units are placed in
`system.slice`, virtual machines and containers registered with `systemd-machined` are
found in `machine.slice`, and user sessions handled by `systemd-logind` in `user.slice`.

## kubelet cgroup driver

The `kubelet` reads and writes to the `cgroup` tree during initial bootstrapping
of the node.  In the future, it write to the `cgroup` tree to satisfy other purposes
around quality of service, etc.

The `kubelet` must cooperate with `systemd` in order to ensure proper function of the
system.  The bootstrapping requirements for a `systemd` system are different than one
without it.

The `kubelet` will accept a new flag to control how it interacts with the `cgroup` tree.

* `--cgroup-driver=` - cgroup driver used by the kubelet. `cgroupfs` or `systemd`.

By default, the `kubelet` should default `--cgroup-driver` to `systemd` on `systemd` distributions.

## kubelet cgroup bootstrapping under systemd

To facilitate understanding, the following block-level architecture will be used to
reference 

The following sections outline how a `systemd` system supports local node accounting
requirements.

###

```
                +--------------+
                | Pod Lifecycle|
                | Manager      |
                +----^----+----+
                     |    |
                     |    |
+----------+       +-+----v---+
|          |       |          |
|  Node    +-------> Container|
|  Manager |       | Manager  |
+---+------+       +-----+----+
    |                    |
    |                    |
    |  +-----------------+
    |  |                 |
    |  |                 |
+---v--v--+        +-----v----+
| cgroups |        | container|
| library |        | runtimes |
+---+-----+        +-----+----+
    |                    |
    |                    |
    +---------+----------+
              |
              |
  +-----------v-----------+
  |     Linux Kernel      |
  +-----------------------+
```

### Node capacity

The `kubelet` will continue to interface with `cAdvisor` to determine node capacity.

### System reserved

The node may set aside a set of designated resources for non-Kubernetes components.

The `kubelet` accepts a flag of the following form to reserve compute resources
for non-Kubernetes components:

* `--system-reserved=:` - A set of `ResourceName`=`ResourceQuantity` pairs that
describe resources reserved for host daemons.

The `kubelet` does not enforce `--system-reserved`, but the ability to distinguish
the static reservation from observed usage is important for node accounting.

The `kubelet` will accept a new flag in a `systemd` environment to support accounting
system-reserved resources.

* `--slice-system="system.slice"` - Slice reserved for
system usage when using systemd cgroup driver.

The `kubelet` will error in any of the following conditions:
* the named `slice` does not exist
* if the `cpu` and `memory` controllers are not enabled on the specified slice

### Kubernetes reserved

The node may set aside a set of resources for Kubernetes components: 

* `--kube-reserved=:` - A set of `ResourceName`=`ResourceQuantity` pairs that
describe resources reserved for host daemons.

The `kubelet` does not enforce `--kube-reserved` at this time, but the ability
to distinguish the static reservation from observed usage is important for node accounting.

This proposal asserts that all components that Kubernetes brings to the node are
parented by a common `slice` in a separate part of the hierarchy than system-reserved
resources.

This proposal asserts that `kubernetes.slice` is the default slice associated with
the `kubelet` and `kube-proxy` service units defined in the project.

The `kubelet` will detect the parent `slice` from which it was launched to track
kubernetes-reserved observed usage.

If the `kubelet` is parented by a `slice` that is categorized as system-reserved,
the `kubelet` will log a warning that kubernetes-reserved observed usage accounting is
disabled, but the static reservation is still observed for purposes of reporting the
allocatable resources for the node.

If the `kubelet` is launched directly from a terminal, it's most likely destination will
be in a `scope` that is a child of `user.slice` as follows:

`/sys/fs/cgroup/<controller>/user.slice/user-1000.slice/session-1.scope`

In this context, the parent `scope` is what will be used to facilitate local developer
debugging scenarios for tracking `kube-reserved` usage.

### Kubernetes container runtime reserved

This proposal asserts that the reservation of compute resources for any associated
container runtime daemons is tracked by the operator under the system-reserved or
kubernetes-reserved values at this time and any enforced limits are set by the
operator specific to the container runtime.

For future consideration, if the `kubelet` attempts to enforce `--kube-reserved`, this
proposal asserts that the reservation associated with any particular container runtime
should be split out at that time.  In a `systemd` environment, this proposal asserts that
you cannot mandate the container runtime daemon is in the same `slice` as the Kubernetes
components.

**Docker**

If the `kubelet` is configured with the `container-runtime` set to `docker`, the
`kubelet` will detect the `service` associated with the `docker` daemon and use that
to do local node accounting.  If an operator wants to impose runtime limits on the
`docker` daemon to control resource usage, the operator should set those explicitly in
the `service` unit that launches `docker`.  The `kubelet` will not set any limits itself
at this time and will assume whatever budget was set aside for `docker` was included in
either `--kube-reserved` or `--system-reserved` reservations.

**rkt**

rkt has no client/server daemon, and therefore has no explicit requirements on container-runtime
reservation.

### Node allocatable

The proposal makes no changes to the definition as presented here:
https://github.com/kubernetes/kubernetes/blob/master/docs/proposals/node-allocatable.md

The node will report a set of allocatable compute resources defined as follows:

`[Allocatable] = [Node Capacity] - [Kube-Reserved] - [System-Reserved]`

### OOM Score Adjustment

The `kubelet` at bootstrapping will set the `oom_score_adj` value for Kubernetes
daemons, and any dependent container-runtime daemons.

If `container-runtime` is set to `docker`, then set its `oom_score_adj=-900`

### Linux Kernel Parameters

The `kubelet` will set the following:

* `sysctl -w vm.overcommit_memory=1`
* `sysctl -w vm.panic_on_oom=0`
* `sysctl -w kernel/panic=10`
* `sysctl -w kernel/panic_on_oops=1`

## kubelet cgroup-root behavior under systemd

The `kubelet` supports a `cgroup-root` flag which is the optional root `cgroup` to use for pods.

This flag should be treated as a pass-through to the underlying configured container runtime.

For example, if the container runtime is `docker` and its using the `systemd` cgroup driver, then
it should take the form of a `systemd` slice.  For example, `--cgroup-root=foo-bar.slice` would parent
all of the pod's container under the `foo-bar.slice` part of the hierarchy.

## kubelet accounting for end-user pods

This proposal re-enforces that it is inappropriate at this time to depend on `--cgroup-root` as the
primary mechanism to distinguish and account for end-user pod compute resource usage.

Instead, the `kubelet` can and should sum the usage of each running `pod` on the node to account for
end-user pod usage separate from system-reserved and kubernetes-reserved accounting via `cAdvisor`.

## Known issues

### Docker runtime support for --cgroup-parent
Docker versions <= 1.0.9 did not have proper support for `-cgroup-parent` flag on `systemd`.  This
was fixed in this PR (https://github.com/docker/docker/pull/18612).  As result, it's expected
that containers launched by the `docker` daemon may continue to go in the default `system.slice` and
appear to be counted under system-reserved node usage accounting.

If operators run with later versions of `docker`, they can avoid this issue via the use of `cgroup-root`
flag on the `kubelet`, but this proposal makes no requirement on operators to do that at this time, and
this can be revisited if/when the project adopts docker 1.10.

### Ability to distinguish user-reserved from system-reserved

It is impractical to assume that all operators will never have to SSH into machines to debug their
own agents on the system.  On `systemd` environments with `pam_systemd`, user sessions are tracked
in `user.slice`. This slice is not currently being monitored by the `kubelet`, and the `kubelet` is
not allowing
any amount of reservation to be made for this `slice` to restrict operator action when directly
on the node. In future iterations of this specification, we may want to introduce reserving a
specific amount of resource on the node for this need separate from `--system-reserved` especially
if the `kubelet` takes an active role in enforcement of the reservation limits.  For example,
if we added a `--slice-user` and a `--user-reserved` flag to `kubelet` as a peer of `slice-system`
and `--system-reserved` we could more effectively monitor and enforce node behavior for any users
that directly log into the system.