# go-http-docker-ansible

Desafio técnico completo envolvendo desenvolvimento de um serviço HTTP em Go, containerização com Docker, proxy reverso com NGINX, monitoramento com Prometheus e Grafana, e automação de infraestrutura com Ansible.

---

## Sumário

- [Visão Geral](#visão-geral)
- [Estrutura do Projeto](#estrutura-do-projeto)
- [Parte 1 - Serviço HTTP + Infraestrutura Docker](#parte-1--serviço-http--infraestrutura-docker)
  - [Serviço Go](#serviço-go)
  - [Dockerfile](#dockerfile)
  - [NGINX como Proxy Reverso](#nginx-como-proxy-reverso)
  - [Docker Compose](#docker-compose)
  - [Rede Docker](#rede-docker)
- [Parte 2 - Monitoramento e Observabilidade](#parte-2--monitoramento-e-observabilidade)
  - [Métricas com Prometheus](#métricas-com-prometheus)
  - [Grafana com Provisionamento Automático](#grafana-com-provisionamento-automático)
- [Parte 3 - Automação com Ansible](#parte-3--automação-com-ansible)
  - [Estrutura do Playbook](#estrutura-do-playbook)
  - [Roles](#roles)
  - [Ambiente Real com AWS EC2](#ambiente-real-com-aws-ec2)
- [Como Executar](#como-executar)
  - [Localmente com Docker Compose](#localmente-com-docker-compose)
  - [Com Ansible](#com-ansible)
- [Decisões Técnicas](#decisões-técnicas)

---

## Visão Geral

O projeto implementa um serviço HTTP chamado `http-server-projeto-korp` que expõe um endpoint `GET /projeto-korp` retornando um JSON com o horário UTC atual resolvido dinamicamente. A infraestrutura é composta por 4 containers orquestrados via Docker Compose, com monitoramento completo e provisionamento automatizado via Ansible.

Fluxo completo:

```
curl localhost:80/projeto-korp
    → NGINX (porta 80)
        → http-server-projeto-korp (porta 8080)
            → {"nome":"Projeto Korp","horario":"2026-06-22T...Z"}

Prometheus (porta 9090)
    → coleta /metrics a cada 15s do serviço Go

Grafana (porta 3000)
    → visualiza métricas do Prometheus
    → dashboard provisionado automaticamente
```

---

## Estrutura do Projeto

```
go-http-docker-ansible/
├── app/
│   ├── main.go
│   ├── go.mod
│   ├── go.sum
│   └── Dockerfile
├── nginx/
│   └── http-server-projeto-korp.conf
├── monitoring/
│   ├── prometheus.yml
│   └── grafana/
│       ├── provisioning/
│       │   ├── datasources/
│       │   │   └── datasources.yml
│       │   └── dashboards/
│       │       └── dashboards.yml
│       └── dashboards/
│           └── http-server-projeto-korp-dashboard.json
├── ansible/
│   ├── site.yml
│   ├── inventory.yml
│   └── roles/
│       ├── docker_setup/
│       │   └── tasks/main.yml
│       ├── app_build/
│       │   └── tasks/main.yml
│       ├── compose_up/
│       │   └── tasks/main.yml
│       └── validate/
│           └── tasks/main.yml
├── docker-compose.yml
└── README.md
```

---

## Parte 1 - Serviço HTTP + Infraestrutura Docker

### Serviço Go

O serviço é escrito em Go puro, sem frameworks externos, usando apenas a biblioteca padrão para o servidor HTTP e `github.com/prometheus/client_golang` para exposição de métricas.

**Endpoint principal:** `GET /projeto-korp`

Resposta:

```json
{
  "nome": "Projeto Korp",
  "horario": "2026-06-22T13:45:00Z"
}
```

O horário é resolvido dinamicamente a cada requisição com `time.Now().UTC().Format(time.RFC3339)`, garantindo que nunca é cacheado.

**Endpoint de métricas:** `GET /metrics`

Expõe todas as métricas no padrão Prometheus para coleta pelo servidor de monitoramento.

---

### Dockerfile

O Dockerfile usa **multi-stage build** para compilação e isolamento.

```dockerfile
FROM golang:1.26-alpine AS builder
# Stage 1: cria usuário non-root e compila o binário Go com ldflags

FROM scratch
# Stage 2: copia usuários e binário para uma imagem vazia (sem shell/SO)
```

**Por que multi-stage e non-root?**

A imagem `golang:1.26-alpine` pesa ~300MB, enquanto a final em `scratch` cai para ~10MB. A compilação utiliza `-ldflags="-s -w"` para descartar lixo de debug. A imagem vazia elimina a superfície de ataque, mas, obrigatoriamente, criamos um `appuser` para evitar que o container rode com o UID 0 (root), garantindo o princípio de menor privilégio.

As flags `CGO_ENABLED=0 GOOS=linux` garantem que o binário é totalmente estático e compatível com Linux, independente de onde foi compilado.

---

### NGINX como Proxy Reverso

O NGINX recebe as requisições externas na porta 80 e encaminha para o serviço Go na porta 8080 via proxy reverso.

```nginx
server {
    listen 80;
    location / {
        proxy_pass http://http-server-projeto-korp:8080;
    }
}
```

O endereço `http-server-projeto-korp` não é um IP - é o nome do container resolvido automaticamente pelo DNS interno do Docker. Todos os containers na mesma rede Docker conseguem se comunicar pelo nome do container.

O arquivo de configuração fica em `nginx/http-server-projeto-korp.conf`. O NGINX carrega automaticamente qualquer arquivo `.conf` dentro de `/etc/nginx/conf.d/` via diretiva `include` no seu arquivo principal. A pasta local é montada nesse caminho via volume no Docker Compose.

> **Nota de Arquitetura e Segurança (Trade-offs assumidos):** Para fins estritos de demonstração da comunicação e facilitação da correção, a configuração do NGINX foi mantida em um estado minimalista (sem timeouts configurados, como `proxy_read_timeout`, e sem *Security Headers* como HSTS ou *X-Frame-Options*). Além disso, no Compose, as portas do Grafana (`3000`) e Prometheus (`9090`) foram mapeadas diretamente ao *host*. Em um cenário produtivo, essas portas devem ser suprimidas e o acesso aos painéis internos de métricas roteado obrigatoriamente através do NGINX (com isolamento na rede do Docker e autenticação HTTP Basic), garantindo Defesa em Profundidade independentemente do *Security Group* da nuvem.

---

### Docker Compose

O `docker-compose.yml` orquestra 4 containers conectados à mesma rede bridge:

| Container | Imagem | Porta exposta ao host |
|---|---|---|
| http-server-projeto-korp | build local | nenhuma |
| nginx-korp | nginx:stable-alpine | 80 |
| prometheus-korp | prom/prometheus:latest | 9090 |
| grafana-korp | grafana/grafana:latest | 3000 |

O serviço Go **não expõe portas ao host** - só é acessível dentro da rede Docker. Todo acesso externo passa obrigatoriamente pelo NGINX.

---

### Rede Docker

A rede `korp-network` é criada no modo `bridge`, que é o modo padrão para comunicação entre containers no mesmo host. O Docker injeta um servidor DNS interno que resolve nomes de containers para seus IPs automaticamente - por isso o NGINX consegue chamar `http://http-server-projeto-korp:8080` sem saber o IP real do container.

---

## Parte 2 - Monitoramento e Observabilidade

### Métricas com Prometheus

O serviço Go exporta métricas e conta com métricas nativas da ferramenta:

**`up` (Métrica Nativa de Disponibilidade)**
Ao invés de hardcodar um Gauge inútil na aplicação, utilizamos a métrica nativa `up` que o próprio *engine* de coleta do Prometheus gera no momento em que tenta raspar o alvo (`1` para sucesso de rede, `0` para falha de comunicação).

**`http_requests_total` (Counter)**
Conta o total de requisições recebidas, com labels `method`, `endpoint` e `status`. Permite filtrar no Grafana por exemplo: "quantas requisições GET em /projeto-korp retornaram 200".

A diferença entre **Gauge** e **Counter**:

- **Gauge** pode subir e descer - usado para valores que variam como disponibilidade, uso de memória, temperatura.
- **Counter** só cresce - usado para contagens acumuladas como total de requisições, erros, bytes transferidos.

O Prometheus é configurado para fazer scrape do endpoint `/metrics` a cada 15 segundos via `prometheus.yml`:

```yaml
scrape_configs:
  - job_name: 'http-server-projeto-korp'
    static_configs:
      - targets: ['http-server-projeto-korp:8080']
```

---

### Grafana com Provisionamento Automático

O Grafana é configurado **100% via arquivos**, sem nenhuma configuração manual pela UI. Isso é o bônus do desafio e garante que qualquer pessoa que provisionar o ambiente via Ansible já terá o dashboard funcionando imediatamente.

São 3 arquivos de provisionamento:

**`datasources.yml`** - conecta o Prometheus como fonte de dados com uid fixo `prometheus-korp`:

```yaml
datasources:
  - name: Prometheus
    type: prometheus
    uid: prometheus-korp
    url: http://prometheus-korp:9090
    isDefault: true
```

O uid fixo é importante: o Grafana gera uids aleatórios quando o datasource é criado pela UI. Definindo no arquivo de provisionamento, garantimos que o uid é sempre `prometheus-korp`, permitindo que o dashboard JSON referencie o datasource corretamente em qualquer ambiente.

**`dashboards.yml`** - diz ao Grafana onde encontrar os JSONs de dashboard:

```yaml
providers:
  - name: 'http-server-projeto-korp'
    folder: 'Korp'
    type: file
    options:
      path: /var/lib/grafana/dashboards
```

**`http-server-projeto-korp-dashboard.json`** - o dashboard com 3 painéis:

- **Disponibilidade do serviço** - exibe UP/DOWN em verde/vermelho baseado na métrica `service_up`
- **Volume de requisições por segundo** - gráfico de linha com `rate(http_requests_total[1m])`
- **Total de requisições** - contador acumulado com `sum(http_requests_total)`

O `rate()` é a função padrão do Prometheus para calcular a velocidade de um counter - "quantas requisições por segundo na última 1 minuto". Usar o valor bruto do counter no Grafana não faz sentido pois ele só cresce; o `rate()` transforma em velocidade legível.

---

## Parte 3 - Automação com Ansible

### Estrutura do Playbook

O ponto de entrada é o `site.yml`, que define a ordem de execução das roles:

```yaml
- name: Provisionar ambiente korp-devops-challenge
  hosts: all
  become: true
  roles:
    - docker_setup
    - app_build
    - compose_up
    - validate
```

O `become: true` equivale ao `sudo` - necessário para instalar pacotes e gerenciar o Docker.

---

### Roles

**`docker_setup`**

Instala o Docker seguindo o procedimento oficial (chave GPG + repositório oficial), inicia o serviço e cria a rede `korp-network`. Todas as tasks são **idempotentes** - podem ser executadas múltiplas vezes sem efeitos colaterais. Por exemplo, a task de adicionar a chave GPG usa `creates:` para verificar se o arquivo já existe antes de rodar.

**`app_build`**

Copia o código da aplicação para `/opt/korp-devops-challenge/app` e builda a imagem Docker usando o módulo `community.docker.docker_image`. O caminho de origem usa `{{ playbook_dir }}` - variável automática do Ansible que aponta para o diretório do `site.yml`, tornando o caminho dinâmico independente de onde o projeto está na máquina.

**`compose_up`**

Copia todos os arquivos do projeto (docker-compose.yml, nginx/, monitoring/) para `/opt/korp-devops-challenge/` e sobe os 4 containers com `community.docker.docker_compose_v2`. O parâmetro `build: never` evita que o Compose tente recompilar a imagem - ela já foi buildada pela role anterior.

**`validate`**

Aguarda a porta 80 ficar disponível com `wait_for`, faz uma requisição HTTP com o módulo `uri` (equivalente ao curl no Ansible), registra a resposta numa variável e exibe no console com `debug`. Output esperado:

```
TASK [validate : Exibir resposta no console]
ok: [localhost] => {
    "msg": "Resposta do serviço: {\"nome\":\"Projeto Korp\",\"horario\":\"2026-06-22T20:54:58Z\"}"
}
```

---

### Ambiente Real com AWS EC2

O playbook foi testado provisionando uma instância EC2 limpa (Ubuntu 24.04) a partir de outra EC2, replicando um cenário real de produção onde o Ansible roda num control node e provisiona servidores remotos via SSH.

O `inventory.yml` referencia a máquina alvo:

```yaml
all:
  hosts:
    korp-server:
      ansible_host: [IP_ADDRESS]
      ansible_user: ubuntu
      ansible_ssh_private_key_file: [PRIVATE_KEY_PATH]
```

**Por que essa abordagem é mais realista que `localhost`?**

O Ansible foi criado para provisionar máquinas remotas. Com `localhost` e `ansible_connection: local` o Ansible executa os comandos diretamente, sem SSH - útil para testes mas não representa o uso real. Usando duas EC2s, a primeira atua como control node e a segunda como managed node, exatamente como em produção onde um servidor de CI/CD (ou sua máquina local) provisiona dezenas de servidores remotos com um único comando.

**Verificação de host SSH**

Antes de rodar o playbook em uma máquina nova, é necessário adicionar o fingerprint SSH ao `known_hosts`:

```bash
ssh-keyscan -H [IP_ADDRESS] >> ~/.ssh/known_hosts
```

Em ambientes com muitas máquinas, isso pode ser automatizado com `ssh-keyscan -f lista-de-ips.txt` ou desabilitando a verificação no `ansible.cfg` com `host_key_checking = False` - prática comum em pipelines CI/CD onde as máquinas são efêmeras.

---

## Como Executar

### Localmente com Docker Compose

Pré-requisitos: Docker e Docker Compose instalados.

```bash
git clone https://github.com/Caua-Vinicius/go-http-docker-ansible.git
cd go-http-docker-ansible
docker compose up --build -d
```

Testar o serviço:

```bash
curl http://localhost:80/projeto-korp
```

Acessar interfaces:

- **Grafana:** <http://localhost:3000> (admin/admin)
- **Prometheus:** <http://localhost:9090>
- **Métricas:** <http://localhost:80/metrics>

---

### Com Ansible

Pré-requisitos: Ansible instalado, acesso SSH à máquina alvo.

1. Edite `ansible/inventory.yml` substituindo os placeholders:
   - `[IP_ADDRESS]` - IP público da máquina alvo
   - `[PRIVATE_KEY_PATH]` - caminho para a chave privada SSH (ex: `~/.ssh/korp-key.pem`)

2. Adicione o fingerprint SSH da máquina alvo:

```bash
ssh-keyscan -H [IP_ADDRESS] >> ~/.ssh/known_hosts
```

1. Instale a coleção community.docker:

```bash
ansible-galaxy collection install community.docker
```

1. Rode o playbook:

```bash
cd ansible
ansible-playbook -i inventory.yml site.yml
```

O ambiente completo será provisionado automaticamente com um único comando. A validação ao final exibirá a resposta do serviço no console confirmando o funcionamento.

---

## Decisões Técnicas

**Go sem frameworks**
A biblioteca padrão do Go é suficiente para um servidor HTTP simples. Usar um framework como Gin ou Echo adicionaria complexidade desnecessária para o escopo do desafio.

**Multi-stage Dockerfile com scratch**
Reduz a imagem de ~300MB para ~10MB. A imagem `scratch` não tem shell nem sistema operacional, o que também melhora a segurança - menos superfície de ataque.

**Provisionamento automático do Grafana**
Em vez de configurar o datasource e dashboard manualmente pela UI, todos os recursos são provisionados via arquivos YAML e JSON. Isso garante que qualquer ambiente provisionado pelo Ansible já tem o dashboard funcionando imediatamente, sem intervenção manual. O uid fixo no datasource (`prometheus-korp`) é o que permite o dashboard JSON referenciar o datasource corretamente entre ambientes.

**Idempotência no Ansible**
Todas as tasks são escritas para serem idempotentes - rodar o playbook múltiplas vezes produz sempre o mesmo resultado sem duplicar recursos ou quebrar o ambiente. Isso é alcançado pela abolição completa de scripts imperativos (`shell`/`command`) em prol de módulos nativos e declarativos (`get_url`, `apt_repository`, `file`), característica essencial de uma infraestrutura escalável.

**Duas EC2s para teste real**
O playbook foi validado provisionando uma EC2 limpa a partir de outra EC2, replicando um cenário real onde o Ansible provisiona servidores remotos via SSH - não apenas localhost.
