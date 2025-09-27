# Useful Scripts for Servers

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

## [Helm](https://helm.sh) installation

Simple run the playbook:

```sh
ansible-playbook playbooks/helm.yaml --ask-become-pass
```

## [Drone CI](https://github.com/drone/charts)

1. Add the Drone Helm Chart repository:

```sh
helm repo add drone https://charts.drone.io
helm repo update
```

2. Go to GitHub Settings -> [Developer Settings](https://github.com/settings/developers) -> OAuth Apps -> New OAuth App.

3. In the form, Homepage URL must match the server IP `http://drone.<domain>` and the callback to the login route `http://drone.<domain>/login`.

4. Set Drone secrets on the server:

```sh
kubectl create secret generic my-drone-secret \
  --namespace drone \
  --from-literal=DRONE_RPC_SECRET=$(openssl rand -hex 16) \
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
  - my-drone-secret
EOF
```

7. Install Drone Server:

```sh
kubectl create namespace drone
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
cat <<-EOF > ./drone-values.yaml
env:
  DRONE_RPC_PROTO: "http"
  DRONE_RPC_HOST: "drone.<domain>"
  DRONE_RUNNER_NAME: "docker-runner"

extraSecretNamesForEnvFrom:
  - my-drone-secret
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
