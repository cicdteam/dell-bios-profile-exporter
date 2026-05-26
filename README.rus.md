# dell-bios-profile-exporter

Prometheus-совместимый экспортер и Helm-чарт для мониторинга атрибута BIOS
**System Profile** на серверах Dell PowerEdge через локальную утилиту `racadm`
(посредством iDRAC Service Module). Формирует алерт при отклонении от целевого
значения (по умолчанию `PerfOptimized`).

## Состав

- `exporter/` - экспортер на Go (контейнер DaemonSet).
- `chart/` - Helm-чарт (DaemonSet, CRD мониторинга, алерты, дашборд).
- `examples/` - готовые values для типовых установок.
- `scripts/verify.sh` - прогон lint/template/unit/kubeconform.

## Сборка образа

```bash
cd exporter
docker build --platform linux/amd64 --build-arg VERSION=0.1.0 \
  -t ghcr.io/cicdteam/dell-bios-profile-exporter:0.1.0 .
```

## Проверка чарта без установки

Чарт работает и с Helm 3.x, и с Helm 4.x.

```bash
helm lint chart/
helm template chart/
helm unittest chart/
./scripts/verify.sh
```

Примечание: под Helm 4 плагин helm-unittest ставится командой
`helm plugin install https://github.com/helm-unittest/helm-unittest --verify=false`.

## Установка в закрытом контуре

```bash
# На машине с доступом в сеть скачайте опубликованный чарт из OCI-реестра:
helm pull oci://ghcr.io/cicdteam/charts/dell-bios-profile-exporter --version 0.1.0
# скопируйте dell-bios-profile-exporter-0.1.0.tgz в закрытую сеть, затем:
helm install dell-bios ./dell-bios-profile-exporter-0.1.0.tgz -f my-values.yaml
```
Образ контейнера нужно отдельно перенести в приватный реестр
(например, через `docker save` / `skopeo copy` в ваш внутренний реестр).

Подробное использование: см. `chart/README.rus.md`.
