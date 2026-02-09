# Useful Tips for Server Management

## Setup

1. Copy setup script for server:

```sh
scp ./setup.sh root@<ip>:/root/
```

2. Allow script execution:

```sh
chmod +x setup.sh
```

3. And run:

```sh
./setup.sh
```

## About [K3s](https://github.com/k3s-io/k3s-ansible)

Just adapt `inventory.yml` to something like this:

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

And then:

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

2. Add the following to `~/.bashrc`:

```sh
export KUBECONFIG=$HOME/.kube/config
```

## [Helm](https://helm.sh) installation

Simple run the playbook:

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

10. Set Drone configurations

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

13. Go to GitHub Settings -> [Developer Settings](https://github.com/settings/developers) -> Personal access tokens -> [Tokens (classic)](https://github.com/settings/tokens) -> [Generate new token (classic)](https://github.com/settings/tokens/new)

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
