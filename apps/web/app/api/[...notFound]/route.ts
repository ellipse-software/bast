import { jsonError } from "@/lib/api-error";

function missing() {
  return jsonError(404, {
    code: "not_found",
    message: "This API route does not exist.",
    hint: "See https://bast.sh/openapi.json and https://bast.sh/developers for the Bast.sh HTTP API.",
  });
}

export function GET() {
  return missing();
}

export function POST() {
  return missing();
}

export function PUT() {
  return missing();
}

export function PATCH() {
  return missing();
}

export function DELETE() {
  return missing();
}
