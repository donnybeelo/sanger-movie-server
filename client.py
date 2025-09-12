import argparse
import requests
from concurrent.futures import ThreadPoolExecutor

verbose = False # Global variable to control verbose output

def main():
	args = cli()

	bearer = authenticate(args)
	if not bearer:
		raise PermissionError("Authentication failed. Username or password incorrect.")

	print_verbose(f"Filtering movies by year(s): {', '.join(map(str, args.year))}")
	for year in args.year:
		numberOfMovies = fetch_movies_by_year(args, year, bearer)
		suffix = "s" if numberOfMovies != 1 else ""
		print(f"Year {year}: {numberOfMovies} movie{suffix}")


# Prints message only if in verbose mode
def print_verbose(message):
	print(message) if verbose else None

# Authenticates with the server using provided username and password, returns bearer token if successful
def authenticate(args):
	print_verbose(f"Connecting to server at {args.server}:{args.port} with username {args.username}")

	# Sends request to server to authenticate with username and password
	try:
		response = requests.post(
			f"http://{args.server}:{args.port}/api/auth",
			json={'username': args.username, 'password': args.password}
		)
	except requests.exceptions.RequestException as e:
		print(f"Failed to connect to server: {e}")
		exit(1)

	bearer = response.json().get('bearer')

	if response.status_code == 200:
		print_verbose("Login successful!")
		return bearer
	else:
		print_verbose(f"Login failed! Status Code: {response.status_code}, Response: {response.text}")
		return None

# Fetches movies in a specific page for a given year, returns a tuple of the number of movies found and the status code
def fetch_movies_in_page(args, year, page, pageCount, bearer):
	# If we've already found an empty page, skip further requests
	if len(pageCount) > 0:
		return 0, 401
	
	response = requests.get(
		f"http://{args.server}:{args.port}/api/movies/{year}/{page}",
		headers={'Authorization': f"Bearer {bearer}"}
	)

	statusCode = response.status_code
	print_verbose(f"Fetching page {page} for year {year}: Status Code {statusCode}")


	if statusCode == 200:
		return len(eval(response.text)), statusCode

	pageCount.append(page)
	return 0, statusCode

# Fetches movies from each page starting from a given page for a specific year until an empty page is found
# or an error code is returned. If a 401 Unauthorized status code is encountered, it re-authenticates and calls itself
def fetch_movies_starting_from_page(args, year, page, bearer):
	pageCount = []

	with ThreadPoolExecutor(max_workers=100) as executor:
		futures = {}
		while len(pageCount) == 0:
			# Submit a request for the current page
			future = executor.submit(fetch_movies_in_page, args, year, page, pageCount, bearer)
			futures[page] = future
			page += 1

		# Wait for all futures to complete
		output = {page:length for page, length in futures.items() if length.result()[1] == 200}

	# Check if the latest non-ok request returned a 401 Unauthorized status code
	authed = futures[min(pageCount)].result()[1] != 401
	if not authed:
		print_verbose("Session expired, re-authenticating...")
		bearer = authenticate(args)
		page = min(pageCount) - 1 if len(pageCount) > 0 else 1
		if page < 1: page = 1
		print_verbose(f"Starting fetch from page {page} for year {year}")
		pageCount = []
		return output | fetch_movies_starting_from_page(args, year, page, bearer)
	return output

# Calls recursive function to fetch all movies for a given year, returns the total number of movies found
def fetch_movies_by_year(args, year, bearer):
	moviePages = fetch_movies_starting_from_page(args, year, 1, bearer)
	return sum(future.result()[0] for future in moviePages.values())

# Establishes parameters, parses the arguments, and returns them as attributes within the `args` object
# Also sets the global `verbose` variable depending on if -v is present
def cli():
	parser = argparse.ArgumentParser(description="Movie Server Client CLI")
	
	# Arguments the client can take
	parser.add_argument(
		'-s',
		'--server',
		required=True,
		type=str,
		help="Server IP address for authentication"
	)
	parser.add_argument(
		'-P',
		'--port',
		type=int,
		default=8080,
		help="Server port for authentication (default: 8080)"
	)
	parser.add_argument(
		'-u',
		'--username',
		required=True,
		type=str,
		help="Username for authentication"
	)
	parser.add_argument(
		'-p',
		'--password',
		required=True,
		type=str,
		help="Password for authentication"
	)
	parser.add_argument(
		'-Y',
		'--year', 
		help="Filter movie database by year",
		required=True,
		type=int,
		action='append'
	)
	parser.add_argument(
		'-v',
		'--verbose',
		help="Enable verbose output",
		required=False,
		action='store_true'
	)

	global verbose
	verbose = parser.parse_args().verbose

	return parser.parse_args()


if __name__ == "__main__":
	main()