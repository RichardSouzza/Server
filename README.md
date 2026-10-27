# Useful Settings for Server Management

## Setup

This script is designed for RHEL-based operating systems and aims to:

- Update system packages;
- Create server access users;
- Harden SSH security—disables root login and password authentication, and enables key-based authentication;
- Configure Fail2Ban—automatically bans upon the first password login attempt.

1. Copy setup script for server:

```sh
scp ./setup.sh root@<ip>:/root/
```

**All commands from this point on must be executed on the server.**

2. Allow script execution:

```sh
chmod +x setup.sh
```

3. And run:

```sh
./setup.sh
```

After the script completes, remember to copy the SSH keys and restart `sshd`.

## [K3s](https://github.com/k3s-io/k3s-ansible) Installation

1. Clone the K3s repository:

```sh
git clone https://github.com/k3s-io/k3s-ansible
```

2. Create the inventory file:

```sh
cp inventory-sample.yml inventory.yml
```

3. Adjust the `inventory.yml` to something like this:

```yml
k3s_cluster:
  children:
    server:
      hosts:
        almalinux:
          ansible_host: <ip>
          ansible_user: ansible
          ansible_become: yes
          ansible_become_method: sudo
          ansible_become_user: root
          ansible_ssh_private_key_file: ~/.ssh/id_ansible

  vars:
    k3s_version: v1.31.12+k3s1
    opt_tls_san:
      - <ip>
      - <domain>
```

4. Run the playbook:

```sh
ansible-playbook playbooks/site.yml -i inventory.yml --ask-become-pass
```

## Kubeconfig

1. Obtain read permission for `kubeconfig`:

```sh
mkdir -p ~/.kube
sudo cp /etc/rancher/k3s/k3s.yaml ~/.kube/config
sudo chown $(id -u):$(id -g) ~/.kube/config
chmod 600 ~/.kube/config
```

2. Set the path for the K3s configuration in `~/.bashrc`:

```sh
export KUBECONFIG=$HOME/.kube/config
```

## [Helm](https://helm.sh) installation

Simple run the playbook from this repository:

```sh
ansible-playbook playbooks/helm.yaml --ask-become-pass
```

## [Drone CI](https://github.com/drone/charts)

1. Add the Drone Helm Chart repository:

```sh
kubectl create namespace drone
helm repo add drone https://charts.drone.io
helm repo update
```

2. Go to GitHub Settings -> [Developer Settings](https://github.com/settings/developers) -> OAuth Apps -> New OAuth App.

3. In the form, Homepage URL must match the server IP `http://drone.<domain>` and the callback to the login route `http://drone.<domain>/login`.

4. Set Drone secrets on the server:

```sh
kubectl create secret generic drone-secrets \
  --namespace drone \
  --from-literal=DRONE_RPC_SECRET=$(openssl rand -hex 16) \
  --from-literal=DRONE_CONFIG_SECRET=$(openssl rand -hex 16) \
  --from-literal=DRONE_GITHUB_CLIENT_ID=<drone_client_id> \
  --from-literal=DRONE_GITHUB_CLIENT_SECRET=<drone_client_secret>
```

### Drone Server

5. Download the chart:

```sh
helm pull drone/drone --untar
```

6. Set Drone configurations:

```sh
cd drone
cat <<-EOF > ./drone-values.yaml
ingress:
  enabled: true
  hosts:
    - host: drone.<domain>
      paths:
        - path: /
          pathType: ImplementationSpecific

env:
  DRONE_SERVER_HOST: "drone.<domain>"
  DRONE_SERVER_PROTO: "http"

extraSecretNamesForEnvFrom:
  - drone-secrets
EOF
```

7. Install Drone Server:

```sh
helm install drone drone/drone \
  --namespace drone \
  --values drone-values.yaml
```

8. When necessary to update:

```sh
helm upgrade drone drone/drone \
  --namespace drone \
  --values drone-values.yaml
```

### Drone Docker Runner

9. Download the chart:

```sh
helm pull drone/drone-runner-docker --untar
```

10. Set Drone configurations:

```sh
cd drone-runner-docker
cat <<-EOF > ./drone-values.yaml
env:
  DRONE_RPC_PROTO: "http"
  DRONE_RPC_HOST: "drone.<domain>"
  DRONE_RUNNER_NAME: "docker-runner"

extraSecretNamesForEnvFrom:
  - drone-secrets
EOF
```

11. Install Drone Docker Runner:

```sh
helm install drone-runner-docker drone/drone-runner-docker \
  --namespace drone \
  --values drone-values.yaml
```

12. When necessary to update:

```sh
helm upgrade drone-runner-docker drone/drone-runner-docker \
  --namespace drone \
  --values drone-values.yaml
```

### [Drone Configuration Extension](https://docs.drone.io/extensions/configuration)

13. Go to GitHub Settings
-> [Developer Settings](https://github.com/settings/developers)
-> Personal access tokens
-> [Tokens (classic)](https://github.com/settings/tokens)
-> [Generate new token (classic)](https://github.com/settings/tokens/new)

14. Select scopes "repo" and "read:packages".

15. Set Container Registry access secrets:

```sh
kubectl create secret docker-registry ghcr-secrets \
  --namespace drone \
  --docker-server=ghcr.io \
  --docker-username=<username> \
  --docker-password=<accessToken>
```

16. To verify the exposed addresses:

```sh
kubectl get ingress -n drone
```

#### MTU fix for pipeline clones:

The Docker network that `drone-runner-docker` creates for each pipeline run does
not inherit the daemon's MTU, which causes `git clone` (and other larger
transfers) inside pipeline steps to fail on this cluster. The fix is a small Go
reverse-proxy sidecar (`proxy.go`, in this repo) that sits between the Runner
and the dind daemon and injects the correct MTU into every
`POST .../networks/create` call.

1. Create a ConfigMap from `proxy.go`:

```sh
kubectl create configmap docker-mtu-proxy -n drone --from-file=proxy.go=./proxy.go
```

2. Edit the `drone-runner-docker` deployment:

```sh
kubectl edit deployment -n drone drone-runner-docker
```

3. Add a volume for the proxy source (alongside the existing `storage` and `daemon-config` volumes):

```yaml
- configMap:
    name: docker-mtu-proxy
  name: mtu-proxy-src
```

4. Add a new sidecar container (alongside `dind`, `gc`, and `drone-runner-docker`):

```yaml
- name: mtu-proxy
  image: golang:1.21-alpine
  command: ["go", "run", "/src/proxy.go"]
  volumeMounts:
  - mountPath: /src
    name: mtu-proxy-src
```

5. Point only the `drone-runner-docker` container (not `dind`, not `gc`) at the
proxy, since it's the only one that creates pipeline networks:

```yaml
- name: DOCKER_HOST
  value: tcp://localhost:2376
```

6. Confirm all 4 containers come up:

```sh
kubectl get pods -n drone -w
```

7. Verify the proxy is intercepting network creation correctly, on the next pipeline run:

```sh
kubectl exec -n drone -it <runner-pod> -c dind -- docker -H tcp://localhost:2375 network ls
kubectl exec -n drone -it <runner-pod> -c dind -- docker -H tcp://localhost:2375 network inspect <drone-network-name>
```

`Options` should show `"com.docker.network.driver.mtu": "1400"`.

Also set the daemon's own `--mtu` flag (used for the default `bridge` network,
e.g. one-off `docker run` debugging) in the `dind` container's `command`:

```yaml
command:
- dockerd
- --config-file=/etc/docker/daemon.json
- --mtu=1400
```

> Note: don't set `mtu` in both `/etc/docker/daemon.json` and the `--mtu` flag
> at the same time — `dockerd` refuses to start if the same directive is set in
> both places, even with matching values.

## Troubleshooting

### Resolution of external domains on a local network

1. Prevent cloud-init from overwriting network configurations:

```sh
sudo vi /etc/cloud/cloud.cfg.d/99-disable-network-config.cfg
```

```yaml
network:
  config: disabled
```

2. Remove `localhost` from the list of DNS search domains:

```sh
sudo vi /etc/resolv.conf
```

```patch
--- /etc/resolv.conf
+++ /etc/resolv.conf
@@ -1,5 +1,4 @@
 ; Created by cloud-init automatically, do not edit.
 ;
-search localhost
 nameserver 1.1.1.1
 nameserver 8.8.4.4
```

3. Reboot to apply the changes:

```sh
$ sudo reboot
```

### Pipeline fails to clone with "Connection reset by peer"

Symptom — a pipeline step fails while cloning, but only on larger
repos/transfers (small requests, like the `drone-config` fetch, succeed):

```sh
+ git clone https://github.com/<org>/<repo> .
Cloning into '.'...
fatal: unable to access 'https://github.com/<org>/<repo>/': Recv failure: Connection reset by peer
```

**Root cause:** `drone-runner-docker` creates a fresh Docker network for every
pipeline run (visible as `drone-<random>` in `docker network ls`), and this
per-run network does not inherit any MTU configured on the `dockerd`
daemon — it always defaults to 1500. On this cluster, k3s's flannel VXLAN
interface (`flannel.1`) has an effective MTU of 1450 (1500 minus VXLAN
encapsulation overhead), so packets larger than that get dropped/reset instead
of properly fragmented. This is a known limitation of CI tools (Drone, GitHub
Actions Runner, GitLab Runner) that talk to the Docker Engine API directly via
SDK rather than shelling out to the `docker` CLI — there's no daemon-level flag
or `daemon.json` key that fixes per-run network MTU, because those only apply
to the default `bridge` network.

Fixed by the MTU proxy sidecar described instead [Drone Configuration Extension, MTU fix for pipeline clones](#mtu-fix-for-pipeline-clones).

Diagnostic trail, in case this regresses (each step rules out one layer):

1. Confirm host/flannel MTU is as expected: `ip link show eth0` (1500) vs
`ip link show flannel.1` (1450) — this gap is normal for k3s VXLAN and is *not*
itself the bug.

2. Confirm `dind`'s default `docker0` bridge has the right MTU:
`kubectl exec -n drone <pod> -c dind -- ip link show docker0`. Note this
reports `NO-CARRIER`/`DOWN` and shows MTU 1500 at rest even when correctly
configured — check `docker network inspect bridge` instead for the real applied
value (`Options."com.docker.network.driver.mtu"`).

3. Rule out the daemon config being stale: a `daemon.json` change
(via ConfigMap) only takes effect on `dockerd` restart, never
hot-reloads — `kubectl rollout restart deployment -n drone drone-runner-docker`
after any change.

4. `mtu` cannot be set in both `--mtu=<n>` (dockerd flag) and `daemon.json` at
the same time — causes `CrashLoopBackOff` with `unable to configure the Docker
daemon with file /etc/docker/daemon.json: the following directives are specified
both as a flag and in the configuration file`. Pick one (the flag is the
reliable one here).

5. The actual bug: inspect the network a real pipeline run creates (not the
default `bridge`, and not a manually-created test network) — `docker network
inspect drone-<id>`. If `Options` is empty, the per-run network has no MTU
override, confirming this issue.

6. `net.ipv4.tcp_mtu_probing=1` (host or container) does **not** fix
this — Path MTU Discovery depends on receiving an ICMP "fragmentation needed"
reply, and that path appears to be black-holed here, so PMTUD never kicks in.
