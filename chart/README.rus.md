# dell-bios-profile-exporter

Helm-чарт, который разворачивает небольшой Prometheus-совместимый экспортер в
виде DaemonSet для наблюдения за настройкой BIOS **System Profile** на серверах
Dell PowerEdge. Он читает значение через локальную утилиту `racadm` (iDRAC
Service Module), отдаёт его как метрики и формирует алерт при отклонении профиля
от целевого значения.

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

## Установка iSM и racadm на узлах

Экспортер ничего не устанавливает на хост; iDRAC Service Module и `racadm` уже
должны присутствовать на каждом узле. Шаги ниже покрывают Ubuntu 22.04 / 24.04 и
Debian 11. Точные имена пакетов зависят от версии OMSA / iSM, поэтому подстройте
их под то, что предоставляет ваш репозиторий или скачанный `.deb`.

Вариант A - добавить apt-репозиторий Dell (узлы с доступом в интернет):

```bash
# Импортируйте GPG-ключ Dell и добавьте репозиторий OMSA/iSM, затем:
sudo apt-get update
sudo apt-get install -y dcism srvadmin-idracadm8
```

Вариант B - скачать `.deb` напрямую (ограниченные сети):

```bash
# Загрузите пакеты dcism и srvadmin-idracadm8 .deb с linux.dell.com
# на машине с доступом, скопируйте их на узел, затем:
sudo dpkg -i dcism_*.deb srvadmin-idracadm8_*.deb
sudo apt-get install -f   # подтянуть недостающие зависимости
```

Включите и запустите демон iDRAC Service Module:

```bash
sudo systemctl enable --now dcismeng
sudo systemctl status dcismeng
```

Убедитесь, что `racadm` может прочитать атрибут, который собирает экспортер:

```bash
racadm get BIOS.SysProfileSettings.SysProfile
# ожидаемый вывод примерно такой:
# [Key=BIOS.Setup.1-1#SysProfileSettings]
# SysProfile=PerfOptimized
```

Если `racadm` находится по другому пути, задайте `exporter.racadmPath`
соответственно.

## Установка чарта

### С kube-prometheus-stack

```bash
helm install dell-bios chart/ -f examples/values-prometheus.yaml -n monitoring
```

Затем проверьте:

- Поды DaemonSet в статусе `Running` на выбранных узлах:
  `kubectl -n monitoring get pods -l app.kubernetes.io/name=dell-bios-profile-exporter -o wide`.
- ServiceMonitor создан и подхвачен Prometheus:
  `kubectl -n monitoring get servicemonitor dell-bios`, и проверьте, что цель
  появилась в Status -> Targets в UI Prometheus. Значение
  `monitoring.additionalLabels` (например, `release: kube-prometheus-stack`)
  должно совпадать с `serviceMonitorSelector` вашего экземпляра Prometheus.

### С k8s-victoria-metrics-stack

```bash
helm install dell-bios chart/ -f examples/values-victoriametrics.yaml -n monitoring
```

Затем проверьте:

- Поды DaemonSet в статусе `Running` (та же команда, что выше).
- VMServiceScrape создан и согласован VictoriaMetrics Operator:
  `kubectl -n monitoring get vmservicescrape dell-bios`, и подтвердите, что цель
  поднята на странице targets в vmagent / VMAgent.

### Мультикластер

Разворачивайте по одному релизу на кластер из одного и того же чарта, меняя лишь
`clusterLabel.value`, чтобы каждая метрика несла отдельную метку `cluster`.
GitOps-раскладка держит один чарт и тонкий overlay values на кластер:

```
clusters/prod-eu/values.yaml   -> clusterLabel.value: prod-eu
clusters/prod-us/values.yaml   -> clusterLabel.value: prod-us
```

Метка `cluster` внедряется через relabeling в ServiceMonitor / VMServiceScrape,
поэтому общий дашборд и алерты могут фильтровать по кластеру. Полный пример см. в
`examples/values-multicluster.yaml`. Во время миграции с Prometheus на
VictoriaMetrics задайте `monitoring.stack: both`, чтобы рендерить оба набора CRD
сразу и старый и новый стеки скрапили параллельно.

## Конфигурация

### Выбор стека мониторинга

- `monitoring.stack`: какие CRD мониторинга рендерить.
  - `prometheus` - kube-prometheus-stack (ServiceMonitor/PodMonitor +
    PrometheusRule).
  - `victoriametrics` - k8s-victoria-metrics-stack (VMServiceScrape/VMPodScrape +
    VMRule).
  - `both` - рендерить оба набора, полезно во время миграции.
  - `none` - не рендерить CRD мониторинга (скрап настраиваете вручную).
- `monitoring.scrapeType`: как собираются метрики.
  - `service` - создать Service плюс ServiceMonitor/VMServiceScrape
    (рекомендуется).
  - `pod` - создать PodMonitor/VMPodScrape и скрапить поды напрямую, без Service.

### Селектор узлов

DaemonSet должен запускаться только на тех узлах, где действительно есть железо
Dell и iDRAC Service Module. Используйте `placement.nodeSelector`,
`placement.tolerations` и `placement.affinity`.

Только воркеры:

```yaml
placement:
  nodeSelector:
    node-role.kubernetes.io/worker: ""
```

Только Dell через метку производителя железа, которую вы ведёте сами:

```yaml
placement:
  nodeSelector:
    hardware-vendor: dell
```

Исключить конкретные nodepool через affinity:

```yaml
placement:
  affinity:
    nodeAffinity:
      requiredDuringSchedulingIgnoredDuringExecution:
        nodeSelectorTerms:
          - matchExpressions:
              - key: nodepool
                operator: NotIn
                values: ["gpu", "edge"]
```

Терпеть taint control-plane (если ваши control-plane узлы тоже Dell и вы хотите их
мониторить):

```yaml
placement:
  tolerations:
    - key: node-role.kubernetes.io/control-plane
      operator: Exists
      effect: NoSchedule
```

### Алерты

Чарт поставляет три правила (рендерятся в PrometheusRule и/или VMRule):

- `DellBiosSysProfileDrift` - текущий профиль отличается от целевого
  (`dell_bios_sys_profile_matches_target == 0`).
- `DellBiosRacadmFailing` - экспортер не смог прочитать значение
  (`dell_bios_racadm_success == 0`); обычно iSM или `racadm` недоступны.
- `DellBiosSysProfileStale` - значение не читалось дольше
  `alerts.rules.stale.maxAgeMinutes`, то есть метрика устарела.

У каждого правила свои `for` и `severity` в `alerts.rules`:

```yaml
alerts:
  enabled: true
  targetProfile: PerfOptimized
  rules:
    drift:
      enabled: true
      for: 15m
      severity: warning
    scrapeFailing:
      enabled: true
      for: 10m
      severity: warning
    stale:
      enabled: true
      maxAgeMinutes: 30
      for: 5m
      severity: warning
```

Маршрутизируйте их через Alertmanager. Минимальный `AlertmanagerConfig`,
сопоставляющий severity `warning`, который выдают эти правила:

```yaml
apiVersion: monitoring.coreos.com/v1alpha1
kind: AlertmanagerConfig
metadata:
  name: dell-bios
  namespace: monitoring
spec:
  route:
    receiver: dell-bios-team
    matchers:
      - name: severity
        value: warning
  receivers:
    - name: dell-bios-team
      # настройте здесь slack/email/webhook
```

### Дашборд Grafana

Установите `dashboard.enabled=true`, чтобы поставить дашборд как ConfigMap с
меткой `grafana_dashboard: "1"`, которую sidecar Grafana автоматически
обнаруживает и импортирует. Дашборд предоставляет переменные: `datasource`,
`cluster`, `node`, `profile` и `target_profile`. Тип источника данных -
`prometheus`; он работает без изменений с VictoriaMetrics, поскольку её источник
данных совместим с PromQL. Используйте `dashboard.annotations` (по умолчанию
`grafana_folder: "Dell Hardware"`), чтобы поместить его в папку Grafana.

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

## Диагностика

Под в `CrashLoopBackOff`:

- Убедитесь, что под действительно запланирован там, где вы ожидаете;
  `nodeSelector`, не совпадающий ни с одним узлом, оставляет DaemonSet с нулём
  подов, а неверный может посадить его на не-Dell узел, где `racadm`
  отсутствует.
- Убедитесь, что security context применён: `security.privileged: true` (или
  capabilities `SYS_ADMIN` / `SYS_PTRACE`, когда privileged выключен) и
  `security.hostPID: true`. Без этого `nsenter` в пространства имён хоста сразу
  падает.
- Убедитесь, что iDRAC Service Module и `racadm` существуют на хосте по
  настроенным путям (`exporter.racadmPath`, `exporter.nsenterPath`).

`dell_bios_racadm_success = 0`:

- iDRAC Service Module не подключён к iDRAC. Проверьте
  `systemctl status dcismeng` на узле.
- Путь к `racadm` неверный или файл неисполняемый. Проверьте
  `exporter.racadmPath`.
- Воспроизведите на самом узле:
  `racadm get BIOS.SysProfileSettings.SysProfile`. Если это падает на хосте, то и
  экспортер не сможет преуспеть.

Пустой дашборд / нет данных:

- Метки скрапа не совпадают с селектором оператора. Проверьте
  `monitoring.additionalLabels` против `serviceMonitorSelector` вашего Prometheus
  / селекторов скрапа VictoriaMetrics.
- Селектор пространства имён оператора не включает namespace, в который вы
  установили чарт.
- Переменная `cluster` дашборда не совпадает с заданным `clusterLabel.value`,
  поэтому каждая панель фильтрует в ничто. Задавайте `clusterLabel.value` на
  каждый кластер.

Периодический `DellBiosRacadmFailing`:

- iDRAC Service Module может ненадолго терять связь с iDRAC, обычно вокруг
  перезагрузок узла или сбросов iDRAC. Короткие провалы ожидаемы; повышайте `for`
  правила, если они шумят, и расследуйте только устойчивые сбои.

Профиль показывает `Custom`:

- `Custom` означает, что хотя бы одна вложенная настройка System Profile была
  изменена относительно значений по умолчанию именованного профиля, поэтому BIOS
  сообщает custom-профиль, а не `PerfOptimized`. Изучите отдельные настройки,
  чтобы понять, что отличается: `racadm get BIOS.SysProfileSettings` на узле и
  сравните под-атрибуты (управление питанием CPU, turbo, C-states, частота памяти
  и т.д.) с целевым профилем.

## Безопасность

### Привилегии

Экспортеру нужны `security.privileged: true` вместе с `security.hostPID: true`,
чтобы он мог сделать `nsenter --target 1` в пространства имён PID и mount хоста и
запустить хостовый `racadm` к сокету iDRAC Service Module. Чтобы уменьшить
поверхность поражения, задайте `security.privileged=false`; тогда чарт переходит к
целевому набору capabilities (`SYS_ADMIN` плюс `SYS_PTRACE`) вместо полной
привилегии. В любом случае под работает как `runAsUser: 0`, потому что вход в
пространства имён хоста требует root.

При Pod Security Standards / Pod Security Admission этот под не может работать в
namespace уровня `restricted`; устанавливайте его в namespace с меткой
`privileged` (например, ваш namespace мониторинга), поскольку `hostPID` и
повышенный security context несовместимы с уровнями `baseline` и `restricted`.

### Что НЕ требуется

- Не нужен сетевой доступ из кластера к iDRAC; всё общение внутриполосное на
  хосте через iDRAC Service Module.
- Не нужны учётные данные iDRAC в Kubernetes.
- Чарт не создаёт и не монтирует Secret.

## Сравнение с альтернативами

| Подход | Как читает профиль | Учётные данные | Сеть до iDRAC |
| --- | --- | --- | --- |
| Этот чарт (локальный `racadm` через iSM) | DaemonSet запускает хостовый `racadm` через iDRAC Service Module | Нет | Нет |
| Сетевой Redfish-экспортер | HTTPS к Redfish API iDRAC | Логин/пароль iDRAC в кластере | Требуется (воркер -> плоскость управления) |
| node_exporter textfile collector | Хостовый cron пишет файл `.prom`, который читает node_exporter | Нет | Нет, но нужен хостовый cron и обвязка файла, которые вы ведёте сами |

## Обновление

```bash
helm upgrade dell-bios chart/ -f my-values.yaml
```

Экземпляры CRD мониторинга (ServiceMonitor/PodMonitor/VMServiceScrape и
PrometheusRule/VMRule) и определения алертов обновляются на месте. Перед сменой
версии чарта просмотрите изменения в `values.yaml` между старой и новой версиями,
чтобы согласовать любые переименованные или удалённые ключи в вашем
`my-values.yaml`.

## Удаление

```bash
helm uninstall dell-bios -n monitoring
```

Это удаляет DaemonSet, Service, ServiceAccount, экземпляры CR мониторинга, алерты
и ConfigMap дашборда. Сами **Определения** CustomResource операторов мониторинга
имеют областью видимости кластер и не создавались этим чартом, поэтому остаются на
месте; удаляются только экземпляры CR, которые этот чарт создал.

## Разработка

```bash
helm lint chart/
helm template chart/
helm unittest chart/
./scripts/verify.sh
```

Сборка образа экспортера:

```bash
cd exporter
docker build --platform linux/amd64 --build-arg VERSION=0.1.5 \
  -t ghcr.io/cicdteam/dell-bios-profile-exporter:0.1.5 .
```

Чарт протестирован против Helm 3.x и Helm 4.x. Под Helm 4 плагин helm-unittest
ставится командой
`helm plugin install https://github.com/helm-unittest/helm-unittest --verify=false`.
