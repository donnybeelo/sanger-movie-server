import unittest
from client import main, authenticate, fetch_movies_by_year
import argparse
import requests

class TestClient(unittest.TestCase):

    def setUp(self):
        # Set up arguments for real server
        self.args = argparse.Namespace(
            server='localhost',
            port=8080, 
            username='username',  
            password='password',
            year=[1894, 1905, 1930, 2020, 2021, 2022, 2023, 2024],
            verbose=False
        )

    def test_server_connection(self):
        # Test connection to server (assuming server is running on localhost:8080)
        try:
            response = requests.get(f"http://{self.args.server}:{self.args.port}/api/movies")
            self.assertEqual(response.content, b'{"error": "year or page not found"}')
        except Exception as e:
            self.fail(f"Connection to server failed: {e}")

    def test_authenticate(self):
        # Call authenticate and verify bearer token is in the correct format (hex:hex)
        bearer = authenticate(self.args)
        self.assertIsInstance(bearer, str)
        self.assertRegex(bearer, r'^[a-fA-F0-9]+:[a-fA-F0-9]+$')

    def test_fetch_movies_by_year(self):
        # Call fetch_movies_by_year and verify output
        bearer = authenticate(self.args)
        total_movies = fetch_movies_by_year(self.args, 1894, bearer)
        self.assertIsInstance(total_movies, int)
        self.assertEqual(total_movies, 1)  # Known number of movies released in 1894
        
        bearer = authenticate(self.args)
        total_movies = fetch_movies_by_year(self.args, 1905, bearer)
        self.assertIsInstance(total_movies, int)
        self.assertEqual(total_movies, 16)  # Known number of movies released in 1905

        bearer = authenticate(self.args)
        total_movies = fetch_movies_by_year(self.args, 1930, bearer)
        self.assertIsInstance(total_movies, int)
        self.assertEqual(total_movies, 1891)  # Known number of movies released in 1930

        bearer = authenticate(self.args)
        total_movies = fetch_movies_by_year(self.args, 2020, bearer)
        self.assertIsInstance(total_movies, int)
        self.assertEqual(total_movies, 14808)  # Known number of movies released in 2020
    
    def test_main(self):
        # Test the main function with the provided args against expected output
        expected = {1894: 1, 1905: 16, 1930: 1891, 2020: 14808, 2021: 17282, 2022: 18513, 2023: 16956, 2024: 14957}
        output = main(self.args)
        self.assertIsInstance(output, dict)
        for year in self.args.year:
            self.assertEqual(output[year], expected[year])


if __name__ == '__main__':
    unittest.main()