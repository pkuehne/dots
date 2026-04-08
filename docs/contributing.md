# Contributing

## Code Style

- dots is a single Python file. Do not split it.
- Python 3.8+ compatible: no f-string walrus operators, no match statements
- No mandatory third-party imports
- Use `.format()` for string formatting (not f-strings, for 3.8 compat clarity)

## Testing

```
pytest tests/
pytest tests/ --cov=dots --cov-report=term-missing
pytest tests/unit/test_config.py -v
```

- No network calls in tests — mock `urllib` and `subprocess.run`
- Each test uses temporary directories via pytest `tmp_path` fixture
- Unit tests in `tests/unit/`, integration tests in `tests/integration/`

## Adding Features

1. Implement in the `dots` file
2. Add unit tests for the new logic
3. Add integration tests for end-to-end behavior
4. Update docs if user-facing
5. Write an ADR if it's a significant design decision
