export type ApiErrorBody = {
  code: string;
  message: string;
  hint: string;
};

export type ApiErrorResponse = ApiErrorBody & {
  error: string;
};

export function apiErrorPayload(error: ApiErrorBody): ApiErrorResponse {
  return {
    error: error.message,
    code: error.code,
    message: error.message,
    hint: error.hint,
  };
}

export function jsonError(
  status: number,
  error: ApiErrorBody,
  headers?: HeadersInit,
): Response {
  return Response.json(apiErrorPayload(error), { status, headers });
}
