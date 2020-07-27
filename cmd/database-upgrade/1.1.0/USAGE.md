# Usage

## Prerequisites
Make sure your node has been upgraded to `v1.0.3`.

## Upgrade Steps

### Step 1 Back up

Back up the database, configured by `db.data_dir` in `config.json`, default `./chain`.

### Step 2 Build
```bash
cd cmd/database-upgrade/1.1.0
make build
```
The build output is `./bin/mass-db-upgrade-1.1.0`

### Step 3 Run 
```bash
./mass-db-upgrade-1.1.0 upgrade [db_dir]
```
`[db_dir]` is the actual value of option `db.data_dir`.

### Step 4 Check (Optional)
```bash
./mass-db-upgrade-1.1.0 check [db_dir]
```
`[db_dir]` is the actual value of option `db.data_dir`.