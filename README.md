# webaction
A web page allows you click a button to execute a script. You can think it as a Jenkins but simplified!

Click a link to execute a task - e.g. ssh to a server to run a command, turn off some light, unlock a door, open a browser, you name it, as long as it is scriptable.

Built in security with basic authentication, works well behind Caddy or other reverse proxy. 
Built in CSRF protection. Mobile friendly.


## Show case
### Task list page
<img width="1728" alt="image" src="https://github.com/user-attachments/assets/9e3946a0-60af-4bc6-bb33-03312c7c8afa" />

### Run Task page
<img width="1728" alt="image" src="https://github.com/user-attachments/assets/94aa4fa8-7a06-4810-b023-1b18bec7fec5" />

### Task Result Page
<img width="1727" alt="image" src="https://github.com/user-attachments/assets/5f115f58-7c6d-4460-8433-b9d45f44bb4f" />

# Building the app

Building is super simple

```bash
go build .
```

The .version file will be used as version in the binary. If you want to regenerate, run the regenerate version shell script before building.

The web action binary will be built. All dependencies are packaged within the built binary.

# Configuration
Refer to `config.yaml.example` for more details

```yaml
listen: 0.0.0.0:8181                                    [1]
csrf:
  secret: password01                                    [2]
  https_only: true
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

# NOTE

If you changed template (for whatever reason), you need to rebuild the binary as templates are packaged into the binary automatically.
# Enjoy
