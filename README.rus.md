# dell-bios-profile-exporter

[![CI](https://github.com/cicdteam/dell-bios-profile-exporter/actions/workflows/ci.yml/badge.svg)](https://github.com/cicdteam/dell-bios-profile-exporter/actions/workflows/ci.yml)
[![License](https://img.shields.io/github/license/cicdteam/dell-bios-profile-exporter)](LICENSE)
[![Go version](https://img.shields.io/github/go-mod/go-version/cicdteam/dell-bios-profile-exporter?filename=exporter/go.mod)](exporter/go.mod)
[![Release](https://img.shields.io/github/v/release/cicdteam/dell-bios-profile-exporter?sort=semver)](https://github.com/cicdteam/dell-bios-profile-exporter/releases/latest)

Prometheus-совместимый экспортер и Helm-чарт для мониторинга атрибута BIOS
**System Profile** на серверах Dell PowerEdge через локальную утилиту `racadm`
(посредством iDRAC Service Module). Формирует алерт при отклонении от целевого
значения (по умолчанию `PerfOptimized`).

## Что делает

DaemonSet запускает небольшой экспортер на Go на каждом выбранном узле. На каждом
опросе экспортер вызывает `racadm` внутри пространств имён PID и mount хоста через
`nsenter`, читает атрибут `BIOS.SysProfileSettings.SysProfile`, кэширует его
(интервал опроса по умолчанию 60s) и отдаёт как метрики Prometheus на порту 9101.
Небольшой набор статической инвентаризации (Service Tag, модель, версия iDRAC)
обновляется с более редким интервалом и добавляется в виде меток.

Алерт срабатывает, когда текущий профиль отклоняется от заданного целевого
значения (по умолчанию `PerfOptimized`), когда экспортер не может прочитать
значение, или когда последнее успешное чтение слишком старое. Поскольку всё
проходит через локальный iDRAC Service Module, экспортеру **не нужен сетевой
доступ к iDRAC и не нужны учётные данные**: он обращается к локальному интерфейсу
управления так же, как это сделал бы администратор на самом хосте.

## Архитектура

```
DaemonSet pod --nsenter--> host racadm --iSM (KCS/USB-NIC)--> iDRAC --> BIOS
```

Почему именно так: запрос к iDRAC по сети через Redfish потребовал бы хранения
учётных данных iDRAC в кластере и сетевой доступности из рабочих узлов в плоскость
управления, что обычно запрещено и здесь сознательно исключено. Утилита `syscfg`
(из Dell Deployment Toolkit) обычно не упакована для Ubuntu, поэтому это не
переносимый вариант. Локальный `racadm`, общающийся с iDRAC Service Module,
использует внутриполосный канал (KCS или внутренний USB NIC), который iDRAC
Service Module уже поддерживает на хосте, поэтому ему не нужен сетевой путь к BMC
и вообще не нужны секреты. Экспортер просто входит в пространства имён хоста и
запускает тот же `racadm`, что и администратор узла.

## Требования

- Kubernetes 1.24+.
- Helm 3.x или 4.x.
- На каждом целевом узле:
  - Сервер Dell PowerEdge, 12-го поколения (12G) или новее.
  - Установленный iDRAC Service Module (`dcism`) с запущенным демоном `dcismeng`,
    подключённым к iDRAC.
  - Доступный на хосте бинарник `racadm` (путь по умолчанию
    `/opt/dell/srvadmin/sbin/racadm`).
- Один уже работающий в кластере стек мониторинга: либо kube-prometheus-stack
  (Prometheus Operator), либо k8s-victoria-metrics-stack (VictoriaMetrics
  Operator).

## Состав

- `exporter/` - экспортер на Go (контейнер DaemonSet).
- `chart/` - Helm-чарт (DaemonSet, CRD мониторинга, алерты, дашборд).
- `examples/` - готовые values для типовых установок.
- `scripts/verify.sh` - прогон lint/template/unit/kubeconform.

## Сборка образа

```bash
cd exporter
docker build --platform linux/amd64 --build-arg VERSION=0.1.5 \
  -t ghcr.io/cicdteam/dell-bios-profile-exporter:0.1.5 .
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
helm pull oci://ghcr.io/cicdteam/charts/dell-bios-profile-exporter --version 0.1.5
# скопируйте dell-bios-profile-exporter-0.1.5.tgz в закрытую сеть, затем:
helm install dell-bios ./dell-bios-profile-exporter-0.1.5.tgz -f my-values.yaml
```
Образ контейнера нужно отдельно перенести в приватный реестр
(например, через `docker save` / `skopeo copy` в ваш внутренний реестр).

## Метрики

| Метрика | Тип | Метки | Значение |
| --- | --- | --- | --- |
| `dell_bios_sys_profile_info` | gauge (=1) | `node`, `profile`, `service_tag`, `model`, `idrac_version` | Info-метрика; текущее значение профиля содержится в метке `profile`. |
| `dell_bios_sys_profile_matches_target` | gauge | `node`, `target` | `1`, если текущий профиль равен целевому, иначе `0`. |
| `dell_bios_racadm_success` | gauge | `node` | `1`, если последний опрос успешен, `0` при сбое. |
| `dell_bios_racadm_duration_seconds` | gauge | `node` | Длительность последнего вызова `racadm`, в секундах. |
| `dell_bios_racadm_errors_total` | counter | `node`, `reason` (`timeout`, `exit_code`, `parse_error`, `nsenter_failed`) | Количество неуспешных вызовов `racadm` по причине. |
| `dell_bios_last_scrape_timestamp_seconds` | gauge | `node` | Unix-время последнего успешного опроса. |
| `dell_bios_exporter_build_info` | gauge (=1) | `version`, `go_version` | Информация о сборке работающего экспортера. |

Предусловия на узлах (установка iSM и racadm), установка чарта, конфигурация,
алерты, дашборд Grafana и troubleshooting - см. `chart/README.rus.md`.
