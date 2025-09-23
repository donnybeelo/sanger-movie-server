To run client:
```bash
	python3 client.py [options]
```
Options:

	--server SERVER      Specify the server URL

	--port PORT          Specify the server port (default: 8080)

	--year YEAR          Specify the year to fetch movies for

	--username USERNAME  Specify the username for authentication

	--password PASSWORD  Specify the password for authentication

	--verbose            Enable verbose output (optional, dramatically decreases speed)
	
Example:

	python3 client.py --server localhost --year 1900 --username user --password pass
