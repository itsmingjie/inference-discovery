"""Check the shared descriptors against the published JSON Schema."""
import json
from pathlib import Path

from jsonschema import Draft202012Validator

root = Path(__file__).resolve().parents[1]
schema = json.loads((root / "spec/schemas/descriptor.schema.json").read_text())
Draft202012Validator.check_schema(schema)
validator = Draft202012Validator(schema)
cases = json.loads((root / "conformance/fixtures/descriptors.json").read_text())
for case in cases:
    # Raw wire cases (e.g. duplicate keys) cannot be checked by JSON Schema.
    if "descriptor" in case:
        assert validator.is_valid(case["descriptor"]) == case["valid"], case["name"]
print("Descriptor schema checks passed")
