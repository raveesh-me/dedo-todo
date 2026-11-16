import { createClient, type Interceptor } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";
import { BoardService } from "../gen/commerce/todo/web/v1/board_pb";
import { authHeaders } from "../session";
import { AppError, isRetired } from "./errors";

/** The commit this frontend was built from; the BFF it calls is pinned to it. */
export const BUILD_SHA: string = import.meta.env.VITE_BUILD_SHA ?? "";

const RELOAD_KEY = `dedo-todo.bff-reloaded.${BUILD_SHA}`;

/** Thrown once this build's BFF is retired and a reload did not help. */
export const RETIRED_MESSAGE =
  "A newer version of this app has been released. Reload the page to continue.";

/**
 * Reloads the page the first time this build's BFF answers that it is
 * retired in this session, which loads the current frontend. Returns false
 * when it already did, or cannot tell, so the caller shows an error instead
 * of reloading in a loop.
 */
function reloadOnce(): boolean {
  try {
    if (window.sessionStorage.getItem(RELOAD_KEY) !== null) return false;
    window.sessionStorage.setItem(RELOAD_KEY, String(Date.now()));
  } catch {
    return false;
  }
  window.location.reload();
  return true;
}

const headers: Interceptor = (next) => async (req) => {
  for (const [k, v] of Object.entries(authHeaders())) req.header.set(k, v);
  return next(req);
};

const retired: Interceptor = (next) => async (req) => {
  try {
    return await next(req);
  } catch (err) {
    if (isRetired(err)) {
      if (reloadOnce()) {
        // The page is going away: never settle, so nothing flashes an error.
        return new Promise<never>(() => {});
      }
      throw new AppError(RETIRED_MESSAGE, "BFF_VERSION_RETIRED");
    }
    throw err;
  }
};

const transport = createConnectTransport({
  baseUrl: `${window.location.origin}/bff/${BUILD_SHA}`,
  useBinaryFormat: false,
  interceptors: [retired, headers],
});

export const board = createClient(BoardService, transport);
