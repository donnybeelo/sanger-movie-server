Movie Server
============

This server serves movie lists via a REST API.

Endpoints
=========

```
The following endpoints are available:

POST /api/auth
	This endpoint allows users to authenticate themselves with the server. Accepts a JSON body with the following format:
		{"username": "USERNAME", "password": "PASSWORD"}

	On a success, the endpoint will return a JSON packet with the following format:
		{"bearer": "TOKEN", "timeout": TOKEN_LIFETIME}

GET /api/movies/$YEAR/$PAGE	
	This endpoint requires the bearer token passed in the Authorization header. Will return a JSON list of upto 10 movies.
```

Usage
=====

```
  -port uint
    	port to listen on (default 8080)
```
