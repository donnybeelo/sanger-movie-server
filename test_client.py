import unittest
from unittest.mock import patch, MagicMock
from client import main, authenticate, fetch_movies_by_year
import argparse

class TestClient(unittest.TestCase):

    @patch('client.authenticate')
    @patch('client.fetch_movies_by_year')
    @patch('client.cli')
    def test_main(self, mock_cli, mock_fetch_movies_by_year, mock_authenticate):
        # Mock CLI arguments
        mock_cli.return_value = argparse.Namespace(
            server='localhost',
            port=8080,
            username='testuser',
            password='testpass',
            year=[1894, 1905, 1930, 2020],
            verbose=True
        )

        # Mock authentication
        mock_authenticate.return_value = 'mock_bearer_token'

        # Mock fetch_movies_by_year
        mock_fetch_movies_by_year.side_effect = [1, 16, 1891, 14808]

        # Capture print statements
        with patch('builtins.print') as mock_print:
            main()

        # Verify prints
        mock_print.assert_any_call("Year 1894: 1 movie")
        mock_print.assert_any_call("Year 1905: 16 movies")
        mock_print.assert_any_call("Year 1930: 1891 movies")
        mock_print.assert_any_call("Year 2020: 14808 movies")

    @patch('client.requests.post')
    def test_authenticate(self, mock_post):
        # Mock response
        mock_response = MagicMock()
        mock_response.status_code = 200
        mock_response.json.return_value = {'bearer': 'mock_bearer_token'}
        mock_post.return_value = mock_response

        # Set up arguments
        global args
        args = argparse.Namespace(
            server='localhost',
            port=8080,
            username='testuser',
            password='testpass'
        )

        # Call authenticate
        bearer = authenticate(args)

        # Verify
        self.assertEqual(bearer, 'mock_bearer_token')
        mock_post.assert_called_once_with(
            'http://localhost:8080/api/auth',
            json={'username': 'testuser', 'password': 'testpass'}
        )

    @patch('client.fetch_movies_starting_from_page')
    def test_fetch_movies_by_year(self, mock_fetch_movies_starting_from_page):
        # Mock response
        mock_fetch_movies_starting_from_page.return_value = {
            1: MagicMock(result=MagicMock(return_value=(1, 200))),
            2: MagicMock(result=MagicMock(return_value=(0, 200)))
        }

        # Call fetch_movies_by_year
        total_movies = fetch_movies_by_year(args, 1894, 'mock_bearer_token')

        # Verify
        self.assertEqual(total_movies, 1)
        mock_fetch_movies_starting_from_page.assert_called_once_with(args, 1894, 1, 'mock_bearer_token')

if __name__ == '__main__':
    unittest.main()