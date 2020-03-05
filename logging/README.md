# MASS Client: Logging Package

## Feature

- Format and output logs to console and file.
- Rotate old log files.

## Usage

### Initialization

```golang
logging.Init(path string, level string, age uint32, disableCPrint bool)
```

- `path`: path of log file.
- `loglevel`: minimum log level to output. `debug`, `info`, `warn`, `error`, `fatal`.
- `age`: time to keep logs. One year by default.

### Logging

```golang
logging.CPrint(level uint32, msg string, formats ...LogFormat) // Output to console and file.
logging.VPrint(level uint32, msg string, formats ...LogFormat) // Output only to file. 
```

- `level`:`debug`, `info`, `warn`, `error`, `fatal`.
  - `error` a call stack will be shown after logged.
  - `fatal` an `exit` signal will be sent to system after logged.
- `msg`: log message.
- `data`: extra content in `LogFormat` (map of {k-v}).

```golang
type LogFormat = map[string]interface{}
```

### Format in log files

`tid` `time` `level` `file` `func` `logline`
