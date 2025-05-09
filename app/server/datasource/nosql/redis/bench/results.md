# Как проводились бенчмарки

- на ВМ без лишних процессов был запущен контейнер с Redis из
  файла [docker-compose.yaml](../../../../../../scripts/bench/docker-compose.yaml)
- затем Redis был заполнен тестовыми данными с помощью
  скрипта [prepareredis.go](../../../../../../scripts/bench/prepareredis.go)
- после чего из корня репозитория выполняем команду
    ```bash
    make build && ./fq-connector-go bench ./scripts/bench/rediscolumns.txt 
    ```
- затем для оценки задержек, вызванных коннектором, проводим бенчмарк максимально простого приложения, выполняющего
  чтение и преобразования аналогичные логике в коннекторе [simpleapp.go](simpleapp.go)

## Результаты

| приложение          | режим  | длина строки, байт | ключей в hash | МБ/сек | строк/сек |
|---------------------|--------|--------------------|---------------|--------|-----------|
| Прямой клиент Redis | string | 10                 |               | 5.36   | 258172    |
| fq-connector-go     | string | 10                 |               | 5.18   | 249433    |
| Прямой клиент Redis | string | 128                |               | 32.08  | 241541    |
| fq-connector-go     | string | 128                |               | 29.83  | 224553    |
| Прямой клиент Redis | string | 512                |               | 102.47 | 205552    |
| fq-connector-go     | string | 512                |               | 93.93  | 188414    |
| Прямой клиент Redis | string | 1024               |               | 165.94 | 168147    |
| fq-connector-go     | string | 1024               |               | 147.31 | 149275    |
| Прямой клиент Redis | string | 2048               |               | 274.84 | 139994    |
| fq-connector-go     | string | 2048               |               | 223.98 | 114090    |
| Прямой клиент Redis | string | 4096               |               | 384.57 | 98195     |
| fq-connector-go     | string | 4096               |               | 284    | 72536     |
| Прямой клиент Redis | string | 8192               |               | 486.67 | 62216     |
| fq-connector-go     | string | 8192               |               | 363.16 | 46376     |
| Прямой клиент Redis | string | 16384              |               | 521.13 | 33332     |
| fq-connector-go     | string | 16384              |               | 463.43 | 29641     |
| Прямой клиент Redis | string | 32768              |               | 650.14 | 20798     |
| fq-connector-go     | string | 32768              |               | 530.38 | 16967     |
| Прямой клиент Redis | string | 65536              |               | 819    | 13103     |
| fq-connector-go     | string | 65536              |               | 541.43 | 8470.14   |
| Прямой клиент Redis | hash   | 10                 | 10            | 25.32  | 146654    |
| fq-connector-go     | hash   | 10                 | 10            | 21.54  | 124714    |
| Прямой клиент Redis | hash   | 100                | 10            | 90.41  | 87716     |
| fq-connector-go     | hash   | 100                | 10            | 87.56  | 84953     |
