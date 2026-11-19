/**
 * The service layer: every API call the application makes is declared here.
 * Components and hooks import from this module rather than calling fetch, so
 * the workspace header, error shape and envelope unwrapping live in one place.
 */
export { ApiError } from "./ApiError";
export { request, setActiveWorkspaceId, getActiveWorkspaceId } from "./client";
export { API_BASE_URL, DEFAULT_PAGE_SIZE, PAGE_SIZE_OPTIONS, WORKSPACE_HEADER } from "./config";
export { productsService } from "./products";
export { orderRequestsService } from "./orderRequests";
export type { OrderRequestListParams } from "./orderRequests";
export { queryKeys } from "./queryKeys";
export { workspacesService } from "./workspaces";
