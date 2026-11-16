import { Code, ConnectError } from "@connectrpc/connect";
import { BinaryReader, WireType } from "@bufbuild/protobuf/wire";

const ERROR_INFO = "google.rpc.ErrorInfo";

/** An error to show the person: what went wrong and its ErrorInfo reason. */
export class AppError extends Error {
  readonly reason: string;
  readonly status: number | undefined;
  constructor(message: string, reason = "", status?: number) {
    super(message);
    this.name = "AppError";
    this.reason = reason;
    this.status = status;
  }
}

/** Decodes the reason of a binary google.rpc.ErrorInfo (field 1). */
function decodeReason(bytes: Uint8Array): string {
  const r = new BinaryReader(bytes);
  while (r.pos < r.len) {
    const [field, wireType] = r.tag();
    if (field === 1 && wireType === WireType.LengthDelimited) {
      return r.string();
    }
    r.skip(wireType, field);
  }
  return "";
}

/** The reason of a Connect error's ErrorInfo detail, or "". */
export function connectReason(err: ConnectError): string {
  for (const d of err.details) {
    if ("type" in d && d.type === ERROR_INFO) {
      try {
        return decodeReason(d.value);
      } catch {
        // A malformed detail: look at the next one.
      }
    }
  }
  return "";
}

export function isRetired(err: unknown): boolean {
  if (!(err instanceof ConnectError)) return false;
  return err.code === Code.FailedPrecondition && connectReason(err) === "BFF_VERSION_RETIRED";
}

/** Turns anything thrown into an AppError. */
export function toAppError(err: unknown): AppError {
  if (err instanceof AppError) return err;
  if (err instanceof ConnectError) {
    return new AppError(err.rawMessage || Code[err.code] || "Request failed", connectReason(err));
  }
  if (err instanceof Error) return new AppError(err.message);
  return new AppError(String(err));
}

/** The text an error banner shows. */
export function describe(err: unknown): string {
  const e = toAppError(err);
  return e.reason ? `${e.message} (${e.reason})` : e.message;
}
