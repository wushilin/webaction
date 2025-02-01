# webaction
A web page allows you click a button to execute a script. You can think it as a Jenkins but simplified!

# Building

Building is super simple

```bash
go build .
```

The web action binary will be built.

# Configuration
Refer to `config.yaml.example` for more details

```yaml
listen: 0.0.0.0:8181                                    [1]
csrf_secret: password01                                 [2]
auth:
  username: admin                                       [3]
  password: password                                    [4]
tasks:
  - name: "No Argument static example"                  [5]
    command:
      - /bin/echo
      - -n
      - "%%Test"
  - name: "Date"                                        [6]
    command:
      - date
  - name: "Ping"                                        [7]
    command:
      - "/sbin/ping"
      - "-c"
      - "%Count"
      - "%Host"
    timeout: 10

  - name: "Echo"                                        [8]
    command:
      - "/bin/echo"
      - "%Message"
    timeout: 2
```

[1] Web Action portal/console bind address
[2] Cross Site Request Foregery secret. Define any string that is strong enough
[3] Web console username (basic auth)
[4] Web console password (basic auth)
[5] An example config that shows a task no argument. %%Test -> Literal string of "%Test". This shows how to pass %xxx as argument
[6] Another example config that shows a task without argument. [5] and [6] are sharing default timeout of 15 seconds
[7] A custom task that takes 2 argument Count and Host. Users will have to enter them on the web UI to run it
[8] A custom task that takes 1 argument Message, and with a custom timeout setting (2 seconds)

# Running

Rename config.yaml.example as config.yaml (optional)

Run with
```bash
./webaction -c config.yaml
```

Access your browser and open http://your-ip:8181 to trigger your tasks

