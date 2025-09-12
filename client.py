import argparse
import requests
from concurrent.futures import ThreadPoolExecutor

verbose = False # Global variable to control verbose output
args = None  # Global variable to hold command-line arguments

def main():
	global args
	args = cli()

	bearer = authenticate()

	print_verbose(f"Filtering movies by year(s): {', '.join(map(str, args.year))}")
	for year in args.year:
		movies = fetch_movies_by_year(year, bearer)
		suffix = "s" if movies != 1 else ""
		print(f"Year {year}: {movies} movie{suffix}")


# Prints message only if in verbose mode
def print_verbose(message):
	if verbose:
		print(message)

def check_authenticated(bearer):
	response = requests.get(
		f"http://{args.server}:{args.port}/api/movies/2025/1",
		headers={'Authorization': f"Bearer {bearer}"}
	)
	return response.status_code != 401


def authenticate():
	print_verbose(f"Connecting to server at {args.server}:{args.port} with username {args.username}")

	# Sends request to server to authenticate with username and password
	response = requests.post(f"http://{args.server}:{args.port}/api/auth", json={'username': args.username, 'password': args.password})

	bearer = response.json().get('bearer')

	if response.status_code == 200:
		print_verbose("Login successful!")
		return bearer
	else:
		print_verbose(f"Login failed! Status Code: {response.status_code}, Response: {response.text}")
		return None


def fetch_movies_by_year(year, bearer):
	errorCode = 0

	def fetch_movies_in_page(year, page, pageCount, bearer):
		nonlocal errorCode

		if len(pageCount) > 0:
			return 0
		response = requests.get(
			f"http://{args.server}:{args.port}/api/movies/{year}/{page}",
			headers={'Authorization': f"Bearer {bearer}"}
		)
		print_verbose(f"Fetching page {page} for year {year}: Status Code {response.status_code}")
		if response.status_code > errorCode:
			errorCode = response.status_code
		if response.status_code == 200:
			# print_verbose(response.text)
			return len(eval(response.text))  # Assuming the server returns a list of movies

		pageCount.append(page)
		return 0

	def fetch_movies_starting_from_page(year, page, bearer):
		nonlocal errorCode
		pageCount = []

		with ThreadPoolExecutor() as executor:
			futures = {}
			while len(pageCount) == 0:
				# Submit a request for the current page
				future = executor.submit(fetch_movies_in_page, year, page, pageCount, bearer)
				futures[page] = future
				page += 1

			# Wait for all futures to complete
			output = {year:length for year, length in futures.items() if length.result() > 0}

		authed = errorCode != 401
		if not authed:
			print_verbose("Session expired, re-authenticating...")
			bearer = authenticate()
			page = min(pageCount) - 1 if len(pageCount) > 0 else 1
			print_verbose(f"Starting fetch from page {page} for year {year}")
			pageCount = []
			return output | fetch_movies_starting_from_page(year, page, bearer)
		return output
			
	moviePages = fetch_movies_starting_from_page(year, 1, bearer)

	return sum(future.result() for future in moviePages.values())


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