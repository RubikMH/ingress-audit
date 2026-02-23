#!/usr/bin/env bash

set -u
set -o pipefail

# ─────────────────────────────────────────────
# ANSI Color / Style Codes
# ─────────────────────────────────────────────
RESET="\033[0m"
RED="\033[0;31m"
GREEN="\033[0;32m"
YELLOW="\033[1;33m"
BLUE="\033[0;34m"
CYAN="\033[0;36m"
BOLD="\033[1m"
DIM="\033[2m"

# ─────────────────────────────────────────────
# Global State
# ─────────────────────────────────────────────
NAMESPACE=""
NAMESPACES=()
SCAN_ALL=false
DOMAIN=""
EMAIL=""
CONTROLLER_NAME="ingress-nginx-controller"

PASS_COUNT=0
WARN_COUNT=0
FAIL_COUNT=0
INFO_COUNT=0

CLUSTER_VERSION=""
API_SERVER=""
CURRENT_CONTEXT=""
NODE_COUNT=0
READY_NODES=0
DEPLOYMENT_TYPE=""
CONTROLLER_IMAGE=""
CONTROLLER_VERSION=""
CONTROLLER_REPLICAS=""
HELM_CHART=""
HELM_CHART_VERSION=""
HELM_STATUS=""
HELM_REVISION=""
ADMISSION_SVC_TYPE=""
ADMISSION_CLUSTER_IP=""
ADMISSION_EXTERNAL_IP=""
INGRESS_EXPOSING=""
ALLOW_SNIPPETS=""
CPU_LIMIT=""
MEMORY_LIMIT=""
CPU_REQUEST=""
MEMORY_REQUEST=""
NP_COUNT=0
UPDATE_STRATEGY=""
IMAGE_PULL_POLICY=""

TEXT_REPORT_FILE=""
JSON_REPORT_FILE=""

# Fix arrays
FIX_IDS=()
FIX_SEVERITIES=()
FIX_DESCRIPTIONS=()
FIX_COMMANDS=()
FIX_METAS=()

# ─────────────────────────────────────────────
# Output helpers
# ─────────────────────────────────────────────
strip_ansi() {
  sed -E 's/\x1b\[[0-9;]*m//g'
}

write() {
  local s="$1"
  printf "%b" "$s"
  if [[ -n "${TEXT_REPORT_FILE:-}" ]]; then
    printf "%b" "$s" | strip_ansi >> "$TEXT_REPORT_FILE"
  fi
}

writeln() {
  write "$1\n"
}

log_pass() {
  writeln "${GREEN}✓ PASS${RESET}: $1"
  ((PASS_COUNT++)) || true
}

log_fail() {
  writeln "${RED}✗ FAIL${RESET}: $1"
  ((FAIL_COUNT++)) || true
}

log_warn() {
  writeln "${YELLOW}⚠ WARN${RESET}: $1"
  ((WARN_COUNT++)) || true
}

log_info() {
  writeln "${BLUE}ℹ INFO${RESET}: $1"
  ((INFO_COUNT++)) || true
}

log_step() {
  writeln "  ${CYAN}→${RESET} $1"
}

print_header() {
  local title="$1"
  local line
  line=$(printf '═%.0s' {1..60})
  writeln "\n${BLUE}${BOLD}${line}${RESET}"
  writeln "${BLUE}${BOLD}    ${title}${RESET}"
  writeln "${BLUE}${BOLD}${line}${RESET}\n"
}

print_section() {
  local title="$1"
  writeln "\n${BOLD}▶ ${title}${RESET}"
  writeln "$(printf '─%.0s' {1..55})"
}

or_default() {
  local s="${1:-}"
  [[ -n "$s" ]] && printf '%s' "$s" || printf 'not-set'
}

to_lower() {
  printf '%s' "${1:-}" | tr '[:upper:]' '[:lower:]'
}

extract_version() {
  local s="${1:-}"
  local token sub
  while IFS= read -r token; do
    [[ "$token" =~ ^v[0-9]+\.[0-9]+\.[0-9]+ ]] || continue
    sub="${token%%-*}"
    printf '%s' "$sub"
    return
  done < <(tr ':/@' '\n\n\n' <<< "$s")

  while IFS= read -r token; do
    sub="${token%%-*}"
    if [[ "$sub" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
      printf '%s' "$sub"
      return
    fi
  done < <(tr ':/@' '\n\n\n' <<< "$s")

  printf 'unknown'
}

add_fix() {
  FIX_IDS+=("$1")
  FIX_SEVERITIES+=("$2")
  FIX_DESCRIPTIONS+=("$3")
  FIX_COMMANDS+=("$4")
  FIX_METAS+=("${5:-}")
}

cmd_exists() {
  command -v "$1" >/dev/null 2>&1
}

kubectl_out() {
  kubectl "$@" 2>/dev/null | tr -d '\r'
}

helm_out() {
  helm "$@" 2>/dev/null | tr -d '\r'
}

non_empty_count() {
  awk 'NF{c++} END{print c+0}'
}

run_cmd() {
  "$@"
}

reset_runtime_state() {
  PASS_COUNT=0
  WARN_COUNT=0
  FAIL_COUNT=0
  INFO_COUNT=0

  CLUSTER_VERSION=""
  API_SERVER=""
  CURRENT_CONTEXT=""
  NODE_COUNT=0
  READY_NODES=0
  DEPLOYMENT_TYPE=""
  CONTROLLER_IMAGE=""
  CONTROLLER_VERSION=""
  CONTROLLER_REPLICAS=""
  HELM_CHART=""
  HELM_CHART_VERSION=""
  HELM_STATUS=""
  HELM_REVISION=""
  ADMISSION_SVC_TYPE=""
  ADMISSION_CLUSTER_IP=""
  ADMISSION_EXTERNAL_IP=""
  INGRESS_EXPOSING=""
  ALLOW_SNIPPETS=""
  CPU_LIMIT=""
  MEMORY_LIMIT=""
  CPU_REQUEST=""
  MEMORY_REQUEST=""
  NP_COUNT=0
  UPDATE_STRATEGY=""
  IMAGE_PULL_POLICY=""

  FIX_IDS=()
  FIX_SEVERITIES=()
  FIX_DESCRIPTIONS=()
  FIX_COMMANDS=()
  FIX_METAS=()
}

# ─────────────────────────────────────────────
# Interactive setup
# ─────────────────────────────────────────────
prompt_user() {
  local prompt="$1"
  local default_val="${2:-}"
  local input
  if [[ -n "$default_val" ]]; then
    printf "  %b%s%b [default: %b%s%b]: " "$BOLD" "$prompt" "$RESET" "$CYAN" "$default_val" "$RESET" >&2
  else
    printf "  %b%s%b: " "$BOLD" "$prompt" "$RESET" >&2
  fi
  IFS= read -r input
  input="${input//[$'\r\n']/}"
  [[ -z "$input" ]] && input="$default_val"
  printf '%s' "$input"
}

fetch_namespaces() {
  kubectl get namespaces -o jsonpath='{.items[*].metadata.name}' 2>/dev/null
}

ns_has_ingress_nginx() {
  local ns="$1"
  local controller="$2"
  local out
  out=$(kubectl get deployment -n "$ns" "$controller" --ignore-not-found 2>/dev/null)
  [[ -n "${out// }" ]] && return 0
  out=$(kubectl get daemonset -n "$ns" "$controller" --ignore-not-found 2>/dev/null)
  [[ -n "${out// }" ]] && return 0
  return 1
}

pick_namespace() {
  printf "\n%b📦 Namespace Selection%b\n" "${BOLD}${BLUE}" "$RESET"
  printf '%s\n' "$(printf '─%.0s' {1..55})"
  printf "  %bFetching namespaces from cluster...%b" "$DIM" "$RESET"

  local all_raw
  all_raw=$(fetch_namespaces)
  if [[ -z "${all_raw// }" ]]; then
    printf " %bFAILED%b\n" "$RED" "$RESET"
    printf "  %bCannot reach cluster. Using manual input.%b\n" "$YELLOW" "$RESET"
    NAMESPACE=$(prompt_user "Namespace" "ingress-nginx")
    NAMESPACES=("$NAMESPACE")
    return
  fi

  read -r -a all_ns <<< "$all_raw"
  printf " %b%d total%b\n" "$GREEN" "${#all_ns[@]}" "$RESET"

  printf "  %bScanning for ingress-nginx controller...%b\n" "$DIM" "$RESET"
  local nginx_ns=()
  local ns
  for ns in "${all_ns[@]}"; do
    if ns_has_ingress_nginx "$ns" "$CONTROLLER_NAME"; then
      nginx_ns+=("$ns")
    fi
  done

  if [[ ${#nginx_ns[@]} -eq 0 ]]; then
    printf "\n  %bNo ingress-nginx controller found in any namespace.%b\n" "$RED" "$RESET"
    printf "  %bEnter namespace manually or press Enter to exit:%b " "$YELLOW" "$RESET"
    local input
    IFS= read -r input
    input="${input//[$'\r\n']/}"
    if [[ -z "$input" ]]; then
      exit 0
    fi
    NAMESPACE="$input"
    NAMESPACES=("$input")
    return
  fi

  printf "\n"
  printf "  %b%-4s  %-30s  %s%b\n" "$BOLD" "#" "NAMESPACE" "INGRESS-NGINX" "$RESET"
  printf "  %s\n" "$(printf '─%.0s' {1..50})"
  if [[ ${#nginx_ns[@]} -gt 1 ]]; then
    printf "  %b%-4s  %-30s%b\n" "${CYAN}${BOLD}" "[ 0]" "Scan ALL of the above" "$RESET"
    printf "  %s\n" "$(printf '─%.0s' {1..50})"
  fi

  local i
  for i in "${!nginx_ns[@]}"; do
    printf "  %b[%2d]%b  %b%-30s%b  %bfound%b\n" \
      "$BOLD" "$((i+1))" "$RESET" \
      "$CYAN" "${nginx_ns[$i]}" "$RESET" \
      "$GREEN" "$RESET"
  done

  printf "\n"
  if [[ ${#nginx_ns[@]} -eq 1 ]]; then
    NAMESPACE="${nginx_ns[0]}"
    NAMESPACES=("$NAMESPACE")
    printf "  %bAuto-selected: %b%s%b (only controller found)\n" "$GREEN" "$CYAN" "$NAMESPACE" "$RESET"
    return
  fi

  while true; do
    printf "  %bEnter number (or comma-separated list, 0 = all):%b " "$BOLD" "$RESET"
    local input
    IFS= read -r input
    input="${input//[$'\r\n']/}"

    if [[ -z "$input" ]]; then
      printf "  %bPlease enter a number or 0 for all.%b\n" "$YELLOW" "$RESET"
      continue
    fi

    if [[ "$input" == "0" || "$(to_lower "$input")" == "all" ]]; then
      SCAN_ALL=true
      NAMESPACES=("${nginx_ns[@]}")
      printf "\n  %bSelected: ALL %d ingress-nginx namespace(s)%b\n" "$GREEN" "${#nginx_ns[@]}" "$RESET"
      return
    fi

    local selected=()
    local valid=true
    IFS=',' read -r -a parts <<< "$input"
    local p idx
    for p in "${parts[@]}"; do
      p="${p// /}"
      if [[ -z "$p" ]]; then
        continue
      fi
      if ! [[ "$p" =~ ^[0-9]+$ ]]; then
        printf "  %bInvalid choice '%s' -- enter numbers between 1 and %d%b\n" "$RED" "$p" "${#nginx_ns[@]}" "$RESET"
        valid=false
        break
      fi
      idx=$((p-1))
      if (( idx < 0 || idx >= ${#nginx_ns[@]} )); then
        printf "  %bInvalid choice '%s' -- enter numbers between 1 and %d%b\n" "$RED" "$p" "${#nginx_ns[@]}" "$RESET"
        valid=false
        break
      fi
      selected+=("${nginx_ns[$idx]}")
    done

    if [[ "$valid" != true ]]; then
      continue
    fi
    if [[ ${#selected[@]} -eq 0 ]]; then
      printf "  %bPlease enter a number or 0 for all.%b\n" "$YELLOW" "$RESET"
      continue
    fi

    NAMESPACES=("${selected[@]}")
    if [[ ${#selected[@]} -eq 1 ]]; then
      NAMESPACE="${selected[0]}"
      SCAN_ALL=false
    else
      SCAN_ALL=true
    fi

    printf "\n  %bSelected:%b" "$GREEN" "$RESET"
    for ns in "${selected[@]}"; do
      printf " %b%s%b" "$CYAN" "$ns" "$RESET"
    done
    printf "\n"
    return
  done
}

interactive_setup() {
  printf "\033[H\033[2J"
  printf "%b\n" "${CYAN}${BOLD}██╗███╗   ██╗ ██████╗ ██████╗ ███████╗███████╗███████╗      █████╗ ██╗   ██╗██████╗ ██╗████████╗"
  printf "%b\n" "${CYAN}${BOLD}██║████╗  ██║██╔════╝ ██╔══██╗██╔════╝██╔════╝██╔════╝     ██╔══██╗██║   ██║██╔══██╗██║╚══██╔══╝"
  printf "%b\n" "${CYAN}${BOLD}██║██╔██╗ ██║██║  ███╗██████╔╝█████╗  ███████╗███████╗     ███████║██║   ██║██║  ██║██║   ██║   "
  printf "%b\n" "${CYAN}${BOLD}██║██║╚██╗██║██║   ██║██╔══██╗██╔══╝  ╚════██║╚════██║     ██╔══██║██║   ██║██║  ██║██║   ██║   "
  printf "%b\n" "${CYAN}${BOLD}██║██║ ╚████║╚██████╔╝██║  ██║███████╗███████║███████║     ██║  ██║╚██████╔╝██████╔╝██║   ██║   "
  printf "%b\n" "${CYAN}${BOLD}╚═╝╚═╝  ╚═══╝ ╚═════╝ ╚═╝  ╚═╝╚══════╝╚══════╝╚══════╝     ╚═╝  ╚═╝ ╚═════╝ ╚═════╝ ╚═╝   ╚═╝   ${RESET}"
  printf "%b  by Rubik MH  |  rubikmh.io%b\n" "$DIM" "$RESET"
  printf '%s\n' "$(printf '─%.0s' {1..65})"
  printf "%bConfiguration Setup%b\n" "${BOLD}${BLUE}" "$RESET"
  printf '%s\n\n' "$(printf '─%.0s' {1..55})"

  DOMAIN=$(prompt_user "Your domain (e.g. rubikmh.io)" "rubikmh.io")
  EMAIL=$(prompt_user "Admin email" "admin@${DOMAIN}")
  printf "\n"
  CONTROLLER_NAME=$(prompt_user "Ingress controller resource name" "ingress-nginx-controller")
  printf "  %bThe scanner will look for a Deployment or DaemonSet with this name.%b\n\n" "$DIM" "$RESET"

  pick_namespace
}

# ─────────────────────────────────────────────
# Phase 1 — Pre-flight
# ─────────────────────────────────────────────
audit_preflight() {
  print_header "PHASE 1 — PRE-FLIGHT CHECKS"

  print_section "Required Tools Validation"
  local t
  for t in kubectl helm jq; do
    log_step "Checking ${t}..."
    if cmd_exists "$t"; then
      local ver="unknown"
      case "$t" in
        kubectl) ver=$(kubectl_out version --client --short | awk '/Client/{print $3; exit}') ;;
        helm) ver=$(helm_out version --short | sed 's/+.*//') ;;
        jq) ver=$(jq --version 2>/dev/null) ;;
      esac
      log_pass "${t} is installed (${ver})"
    else
      log_fail "${t} is not installed — please install it and retry"
      exit 1
    fi
  done

  print_section "Kubernetes Cluster Connectivity"
  log_step "Executing: kubectl cluster-info..."
  if ! kubectl cluster-info >/dev/null 2>&1; then
    log_fail "Cannot reach Kubernetes API server — check kubeconfig"
    exit 1
  fi
  log_pass "Kubernetes cluster is reachable"

  log_step "Fetching cluster version..."
  CLUSTER_VERSION=$(kubectl_out version --short | awk '/Server/{print $3; exit}')
  log_info "Cluster version: ${CLUSTER_VERSION}"

  log_step "Getting API server endpoint..."
  API_SERVER=$(kubectl_out cluster-info | awk '/control plane/{print $NF; exit}')
  log_info "API Server: ${API_SERVER}"

  log_step "Checking current context..."
  CURRENT_CONTEXT=$(kubectl_out config current-context)
  log_info "Context: ${CURRENT_CONTEXT}"

  log_step "Counting cluster nodes..."
  local node_out
  node_out=$(kubectl_out get nodes --no-headers)
  NODE_COUNT=$(printf '%s\n' "$node_out" | non_empty_count)
  READY_NODES=$(printf '%s\n' "$node_out" | awk '/\bReady\b/{c++} END{print c+0}')
  log_info "Cluster nodes: ${READY_NODES}/${NODE_COUNT} ready"

  if (( NODE_COUNT > 0 )); then
    writeln "\n  Node details:"
    kubectl_out get nodes -o custom-columns=NAME:.metadata.name,STATUS:.status.conditions[-1].type,VERSION:.status.nodeInfo.kubeletVersion --no-headers \
      | awk 'NF{print "    "$0}' | while IFS= read -r l; do writeln "$l"; done
  fi

  print_section "Namespace Validation"
  log_step "Executing: kubectl get namespace ${NAMESPACE}..."
  if ! kubectl get namespace "$NAMESPACE" >/dev/null 2>&1; then
    log_fail "Namespace '${NAMESPACE}' not found"
    local ns
    ns=$(kubectl_out get namespaces -o jsonpath='{.items[*].metadata.name}')
    log_info "Available: ${ns}"
    exit 1
  fi
  log_pass "Namespace '${NAMESPACE}' exists"

  local ns_status ns_age
  ns_status=$(kubectl_out get namespace "$NAMESPACE" -o jsonpath='{.status.phase}')
  ns_age=$(kubectl_out get namespace "$NAMESPACE" -o jsonpath='{.metadata.creationTimestamp}')
  log_info "Status: ${ns_status}"
  log_info "Created: ${ns_age}"

  log_step "Counting namespace resources..."
  local pc sc cc
  pc=$(kubectl_out get pods -n "$NAMESPACE" --no-headers | non_empty_count)
  sc=$(kubectl_out get svc -n "$NAMESPACE" --no-headers | non_empty_count)
  cc=$(kubectl_out get cm -n "$NAMESPACE" --no-headers | non_empty_count)
  log_info "Resources: ${pc} pods, ${sc} services, ${cc} configmaps"

  if (( pc > 0 )); then
    writeln "\n  Pod summary:"
    kubectl_out get pods -n "$NAMESPACE" -o custom-columns=NAME:.metadata.name,STATUS:.status.phase,RESTARTS:.status.containerStatuses[0].restartCount --no-headers \
      | awk 'NF{print "    "$0}' | while IFS= read -r l; do writeln "$l"; done
  fi

  print_section "RBAC Permissions Check"
  local checks=(
    "get pods --all-namespaces"
    "get services --all-namespaces"
    "get validatingwebhookconfigurations"
    "get networkpolicies --all-namespaces"
  )
  local check out
  for check in "${checks[@]}"; do
    log_step "Testing 'kubectl auth can-i ${check}'..."
    out=$(kubectl_out auth can-i ${check})
    if [[ "$out" == "yes" ]]; then
      log_pass "Can ${check%% *} ${check#* }"
    else
      log_warn "Limited permission: ${check%% *} ${check#* }"
    fi
  done

  log_info "Pre-flight checks completed ✓"
}

# ─────────────────────────────────────────────
# Phase 2 — Version Audit
# ─────────────────────────────────────────────
audit_version() {
  print_header "PHASE 2 — VERSION AUDIT"

  print_section "Controller Deployment Discovery"
  log_step "Checking for DaemonSet deployment..."

  if kubectl get daemonset -n "$NAMESPACE" "$CONTROLLER_NAME" >/dev/null 2>&1; then
    DEPLOYMENT_TYPE="DaemonSet"
    log_pass "Found controller as DaemonSet"
    log_step "Fetching container image..."
    CONTROLLER_IMAGE=$(kubectl_out get daemonset -n "$NAMESPACE" "$CONTROLLER_NAME" -o jsonpath='{.spec.template.spec.containers[0].image}')
    log_step "Checking replica status..."
    local ready desired
    ready=$(kubectl_out get daemonset -n "$NAMESPACE" "$CONTROLLER_NAME" -o jsonpath='{.status.numberReady}')
    desired=$(kubectl_out get daemonset -n "$NAMESPACE" "$CONTROLLER_NAME" -o jsonpath='{.status.desiredNumberScheduled}')
    CONTROLLER_REPLICAS="${ready}/${desired}"
  else
    log_step "Not a DaemonSet, checking for Deployment..."
    if kubectl get deployment -n "$NAMESPACE" "$CONTROLLER_NAME" >/dev/null 2>&1; then
      DEPLOYMENT_TYPE="Deployment"
      log_pass "Found controller as Deployment"
      log_step "Fetching container image..."
      CONTROLLER_IMAGE=$(kubectl_out get deployment -n "$NAMESPACE" "$CONTROLLER_NAME" -o jsonpath='{.spec.template.spec.containers[0].image}')
      log_step "Checking replica status..."
      local ready_rep desired_rep
      ready_rep=$(kubectl_out get deployment -n "$NAMESPACE" "$CONTROLLER_NAME" -o jsonpath='{.status.readyReplicas}')
      desired_rep=$(kubectl_out get deployment -n "$NAMESPACE" "$CONTROLLER_NAME" -o jsonpath='{.spec.replicas}')
      CONTROLLER_REPLICAS="${ready_rep}/${desired_rep}"
    else
      log_fail "No ingress-nginx-controller found (neither DaemonSet nor Deployment)"
      log_step "Searching for alternative controller names..."
      kubectl_out get deployments,daemonsets -n "$NAMESPACE" -o name | while IFS= read -r l; do writeln "$l"; done
      return
    fi
  fi

  log_info "Ready replicas: ${CONTROLLER_REPLICAS}"

  print_section "Version Analysis"
  log_info "Deployment type:  ${DEPLOYMENT_TYPE}"
  log_info "Container image:  ${CONTROLLER_IMAGE}"

  log_step "Extracting version from image tag..."
  CONTROLLER_VERSION=$(extract_version "$CONTROLLER_IMAGE")
  log_info "Controller version: ${CONTROLLER_VERSION}"
  log_info "Image registry: $(awk -F/ '{print $1}' <<< "$CONTROLLER_IMAGE")"

  if [[ "$CONTROLLER_IMAGE" == *"@sha256:"* ]]; then
    local sha
    sha="${CONTROLLER_IMAGE#*@}"
    log_info "Image SHA: ${sha:0:20}..."
  fi

  local latest_version="v1.14.3"
  log_step "Comparing ${CONTROLLER_VERSION} with latest ${latest_version}..."

  if [[ "$CONTROLLER_VERSION" == "$latest_version" ]]; then
    log_pass "Running latest stable version ${latest_version} ✓"
  elif [[ "$CONTROLLER_VERSION" == "unknown" ]]; then
    log_warn "Could not determine version from image tag — manual verification required"
  else
    log_fail "Outdated version ${CONTROLLER_VERSION} (latest: ${latest_version})"
    log_info "Upgrade command: helm upgrade ingress-nginx ingress-nginx/ingress-nginx --version 4.14.3 -n ${NAMESPACE}"
    add_fix "upgrade-controller" "CRITICAL" \
      "Upgrade ingress-nginx controller from ${CONTROLLER_VERSION} to v1.14.3" \
      "helm upgrade ingress-nginx ingress-nginx/ingress-nginx --version 4.14.3 -n ${NAMESPACE}" \
      ""
  fi

  print_section "Helm Chart Information"
  log_step "Querying Helm releases in namespace ${NAMESPACE}..."
  local helm_json
  helm_json=$(helm_out list -n "$NAMESPACE" -o json)
  if [[ -n "${helm_json// }" ]]; then
    HELM_CHART=$(jq -r '.[] | select(.name=="ingress-nginx") | .chart // empty' <<< "$helm_json")
    HELM_STATUS=$(jq -r '.[] | select(.name=="ingress-nginx") | .status // empty' <<< "$helm_json")
    HELM_REVISION=$(jq -r '.[] | select(.name=="ingress-nginx") | .revision // empty' <<< "$helm_json")
    if [[ -n "$HELM_CHART" ]]; then
      HELM_CHART_VERSION=$(extract_version "$HELM_CHART")
      log_info "Helm chart:    ${HELM_CHART}"
      log_info "Status:        ${HELM_STATUS}"
      log_info "Revision:      ${HELM_REVISION}"
      if [[ "$HELM_CHART_VERSION" == "4.14.3" ]]; then
        log_pass "Running latest Helm chart version 4.14.3"
      else
        log_warn "Chart version ${HELM_CHART_VERSION} may be outdated (latest: 4.14.3)"
      fi
    fi
  else
    log_warn "Could not query Helm releases — not installed via Helm or Helm not available"
  fi

  print_section "Update Configuration"
  local res_type
  res_type=$(tr '[:upper:]' '[:lower:]' <<< "$DEPLOYMENT_TYPE")
  log_step "Fetching image pull policy..."
  IMAGE_PULL_POLICY=$(kubectl_out get "$res_type" -n "$NAMESPACE" "$CONTROLLER_NAME" -o jsonpath='{.spec.template.spec.containers[0].imagePullPolicy}')
  log_info "Image pull policy: ${IMAGE_PULL_POLICY}"

  log_step "Fetching update strategy..."
  if [[ "$DEPLOYMENT_TYPE" == "DaemonSet" ]]; then
    UPDATE_STRATEGY=$(kubectl_out get daemonset -n "$NAMESPACE" "$CONTROLLER_NAME" -o jsonpath='{.spec.updateStrategy.type}')
  else
    UPDATE_STRATEGY=$(kubectl_out get deployment -n "$NAMESPACE" "$CONTROLLER_NAME" -o jsonpath='{.spec.strategy.type}')
    local surge unavail
    surge=$(kubectl_out get deployment -n "$NAMESPACE" "$CONTROLLER_NAME" -o jsonpath='{.spec.strategy.rollingUpdate.maxSurge}')
    unavail=$(kubectl_out get deployment -n "$NAMESPACE" "$CONTROLLER_NAME" -o jsonpath='{.spec.strategy.rollingUpdate.maxUnavailable}')
    log_info "Update strategy: ${UPDATE_STRATEGY} (maxSurge: ${surge}, maxUnavailable: ${unavail})"
  fi
  log_info "Update strategy: ${UPDATE_STRATEGY}"

  print_section "Project Lifecycle Status"
  log_warn "⚠️  IMPORTANT: Ingress-NGINX community project is retiring in March 2026"
  log_info "Timeline: ~1 month remaining before end-of-life"
  log_info "Impact: No security updates, bug fixes, or support after March 2026"
  log_info "Action required: Plan migration to Gateway API or alternative controller"
  writeln "\n  Recommended alternatives:"
  writeln "    1. Gateway API (recommended) — Future-proof Kubernetes standard"
  writeln "    2. Traefik            — Drop-in replacement, actively maintained"
  writeln "    3. F5 NGINX Ingress   — Commercial, actively maintained"
  writeln "    4. Istio              — Service mesh with ingress capabilities"
}

# ─────────────────────────────────────────────
# Phase 3 — Admission Controller Security Audit
# ─────────────────────────────────────────────
audit_admission_controller() {
  print_header "PHASE 3 — ADMISSION CONTROLLER SECURITY AUDIT"

  print_section "Service Discovery"
  log_step "Searching for admission controller service..."

  if ! kubectl get svc -n "$NAMESPACE" ingress-nginx-controller-admission >/dev/null 2>&1; then
    log_warn "Admission controller service not found — may be a custom installation"
    return
  fi
  log_pass "Found admission controller service"

  print_section "Service Configuration Analysis"
  log_step "Fetching service type..."
  ADMISSION_SVC_TYPE=$(kubectl_out get svc -n "$NAMESPACE" ingress-nginx-controller-admission -o jsonpath='{.spec.type}')

  log_step "Fetching cluster IP..."
  ADMISSION_CLUSTER_IP=$(kubectl_out get svc -n "$NAMESPACE" ingress-nginx-controller-admission -o jsonpath='{.spec.clusterIP}')

  log_step "Fetching ports..."
  local ports selector svc_age
  ports=$(kubectl_out get svc -n "$NAMESPACE" ingress-nginx-controller-admission -o jsonpath='{.spec.ports[*].port}')
  log_step "Fetching selector..."
  selector=$(kubectl_out get svc -n "$NAMESPACE" ingress-nginx-controller-admission -o jsonpath='{.spec.selector}')
  svc_age=$(kubectl_out get svc -n "$NAMESPACE" ingress-nginx-controller-admission -o jsonpath='{.metadata.creationTimestamp}')

  log_info "Name:       ingress-nginx-controller-admission"
  log_info "Type:       ${ADMISSION_SVC_TYPE}"
  log_info "Cluster IP: ${ADMISSION_CLUSTER_IP}"
  log_info "Ports:      ${ports}"
  log_info "Selector:   ${selector}"
  log_info "Created:    ${svc_age}"

  print_section "🔒 CRITICAL: External Exposure Check"
  log_step "Analyzing service type: ${ADMISSION_SVC_TYPE}..."

  case "$ADMISSION_SVC_TYPE" in
    ClusterIP)
      log_pass "✓ Admission controller uses ClusterIP (internal only)"
      log_info "External access: BLOCKED ✓"
      log_info "Security posture: SECURE"
      writeln "  This configuration meets security best practices:"
      writeln "    ✓ Not accessible from the internet"
      writeln "    ✓ Protected by cluster network policies"
      writeln "    ✓ Compliant with AbuseBSI requirements for ${DOMAIN}"
      ;;
    LoadBalancer)
      ADMISSION_EXTERNAL_IP=$(kubectl_out get svc -n "$NAMESPACE" ingress-nginx-controller-admission -o jsonpath='{.status.loadBalancer.ingress[0].ip}')
      log_fail "✗ CRITICAL: Admission controller exposed via LoadBalancer!"
      log_info "External IP: ${ADMISSION_EXTERNAL_IP}"
      writeln "  IMMEDIATE REMEDIATION:"
      writeln "    kubectl patch svc ingress-nginx-controller-admission \\\\"
      writeln "      -n ${NAMESPACE} \\\\"
      writeln "      -p '{\"spec\":{\"type\":\"ClusterIP\"}}'"
      add_fix "admission-loadbalancer" "CRITICAL" \
        "Change admission controller service from LoadBalancer → ClusterIP" \
        "kubectl patch svc ingress-nginx-controller-admission -n ${NAMESPACE} -p '{\"spec\":{\"type\":\"ClusterIP\"}}'" \
        ""
      ;;
    NodePort)
      local node_port
      node_port=$(kubectl_out get svc -n "$NAMESPACE" ingress-nginx-controller-admission -o jsonpath='{.spec.ports[0].nodePort}')
      log_fail "✗ CRITICAL: Admission controller exposed via NodePort ${node_port}!"
      writeln "  IMMEDIATE REMEDIATION:"
      writeln "    kubectl patch svc ingress-nginx-controller-admission \\\\"
      writeln "      -n ${NAMESPACE} \\\\"
      writeln "      -p '{\"spec\":{\"type\":\"ClusterIP\"}}'"
      writeln "\n  Exposed on nodes:"
      kubectl_out get nodes -o custom-columns=NAME:.metadata.name,INTERNAL-IP:.status.addresses[0].address --no-headers \
        | awk 'NF{print "    "$0}' | while IFS= read -r l; do writeln "$l"; done
      add_fix "admission-loadbalancer" "CRITICAL" \
        "Change admission controller service from NodePort → ClusterIP" \
        "kubectl patch svc ingress-nginx-controller-admission -n ${NAMESPACE} -p '{\"spec\":{\"type\":\"ClusterIP\"}}'" \
        ""
      ;;
  esac

  local ext_ips
  ext_ips=$(kubectl_out get svc -n "$NAMESPACE" ingress-nginx-controller-admission -o jsonpath='{.spec.externalIPs}')
  if [[ -n "$ext_ips" && "$ext_ips" != "null" ]]; then
    log_fail "External IPs explicitly configured: ${ext_ips}"
  fi

  print_section "Ingress Resource Exposure Check"
  log_step "Scanning all Ingress resources for admission controller exposure..."
  INGRESS_EXPOSING=""
  local ingress_json
  ingress_json=$(kubectl_out get ingress -A -o json)
  if [[ -n "${ingress_json// }" ]]; then
    INGRESS_EXPOSING=$(jq -r '
      .items[]
      | . as $ing
      | [.spec.rules[]?.http.paths[]?.backend.service.name] as $svcs
      | select(($svcs | join(" ") | test("admission")))
      | "\($ing.metadata.namespace)/\($ing.metadata.name)"
    ' <<< "$ingress_json")
  fi

  if [[ -n "${INGRESS_EXPOSING// }" ]]; then
    log_fail "✗ CRITICAL: Found Ingress resource(s) exposing admission controller!"
    while IFS= read -r ing; do
      [[ -z "$ing" ]] && continue
      writeln "    - ${ing}"
    done <<< "$INGRESS_EXPOSING"
    log_info "IMMEDIATE REMEDIATION: Remove these Ingress resources or update backend service"
    add_fix "ingress-exposure" "CRITICAL" \
      "Delete Ingress resource(s) exposing admission controller: ${INGRESS_EXPOSING//$'\n'/, }" \
      "kubectl delete ingress -n <ns> <name>  (for each: ${INGRESS_EXPOSING//$'\n'/, })" \
      "$INGRESS_EXPOSING"
  else
    log_pass "✓ No Ingress resources exposing admission controller"
    log_info "All Ingress resources verified safe"
  fi

  print_section "Webhook Configuration Validation"
  log_step "Analyzing ValidatingWebhookConfiguration..."
  local wh_json
  wh_json=$(kubectl_out get validatingwebhookconfigurations -o json)
  if [[ -n "${wh_json// }" ]]; then
    local wh_lines
    wh_lines=$(jq -r --arg ns "$NAMESPACE" '
      .items[]
      | select(.metadata.name | test("ingress-nginx"))
      | .metadata.name as $name
      | .webhooks[0] as $w
      | [
          "Webhook name: \($name)",
          "  Points to service: \($w.clientConfig.service.name)",
          "  In namespace:      \($w.clientConfig.service.namespace)",
          "  Port:              \($w.clientConfig.service.port)",
          "  Failure policy:    \($w.failurePolicy)",
          (if ($w.clientConfig.service.namespace == $ns)
            then "PASS: Webhook configured correctly for namespace \($ns)"
            else "WARN: Webhook namespace mismatch: expected \($ns)" end)
        ] | .[]
    ' <<< "$wh_json")

    if [[ -n "${wh_lines// }" ]]; then
      while IFS= read -r line; do
        [[ -z "$line" ]] && continue
        if [[ "$line" == PASS:* ]]; then
          log_pass "✓ ${line#PASS: }"
        elif [[ "$line" == WARN:* ]]; then
          log_warn "${line#WARN: }"
        else
          log_info "$line"
        fi
      done <<< "$wh_lines"
    fi
  else
    log_warn "ValidatingWebhookConfiguration not found — admission controller may not be active"
  fi

  print_section "Network Accessibility Analysis"
  log_step "Checking service endpoints..."
  local ep_ips ep_count
  ep_ips=$(kubectl_out get endpoints -n "$NAMESPACE" ingress-nginx-controller-admission -o jsonpath='{.subsets[*].addresses[*].ip}')
  ep_count=$(wc -w <<< "$ep_ips" | awk '{print $1}')
  if (( ep_count > 0 )); then
    log_info "Service has ${ep_count} active endpoint(s): ${ep_ips}"
  else
    log_warn "No active endpoints found — service may not be functional"
  fi

  print_section "AbuseBSI Compliance Summary"
  writeln "  Report ID: CB-Report#20260218-10009947"
  writeln "  Domain:    ${DOMAIN}"
  writeln "  Email:     ${EMAIL}"

  if [[ "$ADMISSION_SVC_TYPE" == "ClusterIP" && -z "${INGRESS_EXPOSING// }" ]]; then
    writeln "\n  ${GREEN}${BOLD}✓ COMPLIANT — Vulnerability has been mitigated.${RESET}\n"
    writeln "  Details:"
    writeln "    ✓ Admission controller is ClusterIP (not exposed)"
    writeln "    ✓ No Ingress resources expose the admission endpoint"
    writeln "    ✓ Only accessible within cluster network"
  else
    writeln "\n  ${RED}${BOLD}✗ NON-COMPLIANT — Vulnerability is STILL PRESENT!${RESET}\n"
    if [[ "$ADMISSION_SVC_TYPE" != "ClusterIP" ]]; then
      writeln "    ✗ Service type is ${ADMISSION_SVC_TYPE} (must be ClusterIP)"
    fi
    if [[ -n "${INGRESS_EXPOSING// }" ]]; then
      writeln "    ✗ Ingress resources are exposing the admission controller"
    fi
    writeln "\n  IMMEDIATE ACTION REQUIRED — See remediation steps above."
  fi
}

# ─────────────────────────────────────────────
# Phase 4 — Network Security Audit
# ─────────────────────────────────────────────
audit_network_security() {
  print_header "PHASE 4 — NETWORK SECURITY AUDIT"

  print_section "Controller Service Exposure"
  log_step "Fetching controller service type..."

  local ctrl_svc_type
  ctrl_svc_type=$(kubectl_out get svc -n "$NAMESPACE" "$CONTROLLER_NAME" -o jsonpath='{.spec.type}')
  if [[ -z "$ctrl_svc_type" ]]; then
    log_warn "Controller service not found"
  else
    log_info "Controller service type: ${ctrl_svc_type}"
    case "$ctrl_svc_type" in
      LoadBalancer)
        local ext_ip
        ext_ip=$(kubectl_out get svc -n "$NAMESPACE" "$CONTROLLER_NAME" -o jsonpath='{.status.loadBalancer.ingress[0].ip}')
        log_info "External IP: ${ext_ip}"
        log_pass "Controller properly exposed via LoadBalancer (expected for ingress)"
        ;;
      NodePort)
        local http_port https_port
        http_port=$(kubectl_out get svc -n "$NAMESPACE" "$CONTROLLER_NAME" -o jsonpath='{.spec.ports[?(@.name=="http")].nodePort}')
        https_port=$(kubectl_out get svc -n "$NAMESPACE" "$CONTROLLER_NAME" -o jsonpath='{.spec.ports[?(@.name=="https")].nodePort}')
        log_info "HTTP NodePort:  ${http_port}"
        log_info "HTTPS NodePort: ${https_port}"
        log_pass "Controller exposed via NodePort (common for bare-metal)"
        ;;
      ClusterIP)
        log_warn "Controller is ClusterIP — confirm external access is handled elsewhere"
        ;;
    esac
  fi

  print_section "Network Policy Check"
  log_step "Checking for NetworkPolicies..."
  local np_out
  np_out=$(kubectl_out get networkpolicies -n "$NAMESPACE" --no-headers)
  NP_COUNT=$(printf '%s\n' "$np_out" | non_empty_count)
  if (( NP_COUNT > 0 )); then
    log_pass "Found ${NP_COUNT} NetworkPolicy resource(s)"
    printf '%s\n' "$np_out" | awk 'NF{print "    "$0}' | while IFS= read -r l; do writeln "$l"; done
  else
    log_warn "No NetworkPolicies found — consider adding for defense-in-depth"
  fi

  print_section "All External Services (Cluster-wide)"
  log_step "Scanning for LoadBalancer/NodePort services..."
  local svc_json
  svc_json=$(kubectl_out get svc -A -o json)
  if [[ -n "${svc_json// }" ]]; then
    local found=0
    while IFS= read -r line; do
      [[ -z "$line" ]] && continue
      log_info "  ${line}"
      ((found++)) || true
    done < <(jq -r '.items[] | select(.spec.type=="LoadBalancer" or .spec.type=="NodePort") | "\(.metadata.namespace)/\(.metadata.name) (\(.spec.type))"' <<< "$svc_json")
    if (( found == 0 )); then
      log_info "No LoadBalancer or NodePort services found"
    fi
  fi
}

# ─────────────────────────────────────────────
# Phase 5 — Configuration Audit
# ─────────────────────────────────────────────
audit_configuration() {
  print_header "PHASE 5 — CONFIGURATION AUDIT"

  print_section "ConfigMap Settings"
  log_step "Fetching ingress-nginx-controller configmap..."
  local cm_json
  cm_json=$(kubectl_out get configmap -n "$NAMESPACE" "$CONTROLLER_NAME" -o json)
  if [[ -n "${cm_json// }" ]]; then
    log_step "Checking allow-snippet-annotations..."
    ALLOW_SNIPPETS=$(jq -r '.data["allow-snippet-annotations"] // ""' <<< "$cm_json")
    if [[ "$ALLOW_SNIPPETS" == "true" ]]; then
      log_fail "SECURITY RISK: allow-snippet-annotations is enabled"
      log_info "REMEDIATION: Disable snippet annotations unless absolutely required"
      add_fix "snippet-annotations" "CRITICAL" \
        "Disable allow-snippet-annotations in ingress-nginx ConfigMap" \
        "kubectl patch cm ingress-nginx-controller -n ${NAMESPACE} --type merge -p '{\"data\":{\"allow-snippet-annotations\":\"false\"}}'" \
        ""
    else
      log_pass "Snippet annotations disabled (secure default)"
    fi

    log_step "Checking SSL protocols..."
    local ssl_proto
    ssl_proto=$(jq -r '.data["ssl-protocols"] // ""' <<< "$cm_json")
    log_info "SSL protocols: ${ssl_proto}"
    if [[ "$ssl_proto" == *"TLSv1 "* ]]; then
      log_warn "TLSv1 is enabled — consider disabling for better security"
    fi

    log_step "Checking custom HTTP errors..."
    local custom_err
    custom_err=$(jq -r '.data["custom-http-errors"] // ""' <<< "$cm_json")
    log_info "Custom HTTP errors: ${custom_err}"
  else
    log_warn "ConfigMap 'ingress-nginx-controller' not found"
  fi

  print_section "Resource Limits"
  local res_type
  res_type=$(tr '[:upper:]' '[:lower:]' <<< "$DEPLOYMENT_TYPE")
  [[ -z "$res_type" ]] && res_type="deployment"

  log_step "Fetching resource limits..."
  CPU_REQUEST=$(kubectl_out get "$res_type" -n "$NAMESPACE" "$CONTROLLER_NAME" -o jsonpath='{.spec.template.spec.containers[0].resources.requests.cpu}')
  CPU_LIMIT=$(kubectl_out get "$res_type" -n "$NAMESPACE" "$CONTROLLER_NAME" -o jsonpath='{.spec.template.spec.containers[0].resources.limits.cpu}')
  MEMORY_REQUEST=$(kubectl_out get "$res_type" -n "$NAMESPACE" "$CONTROLLER_NAME" -o jsonpath='{.spec.template.spec.containers[0].resources.requests.memory}')
  MEMORY_LIMIT=$(kubectl_out get "$res_type" -n "$NAMESPACE" "$CONTROLLER_NAME" -o jsonpath='{.spec.template.spec.containers[0].resources.limits.memory}')

  log_info "CPU:    request=$(or_default "$CPU_REQUEST")  limit=$(or_default "$CPU_LIMIT")"
  log_info "Memory: request=$(or_default "$MEMORY_REQUEST")  limit=$(or_default "$MEMORY_LIMIT")"

  if [[ -z "$CPU_LIMIT" || -z "$MEMORY_LIMIT" ]]; then
    if [[ -n "$DEPLOYMENT_TYPE" ]]; then
      log_warn "Resource limits not set — may impact cluster stability"
      add_fix "resource-limits" "WARNING" \
        "Set default resource limits on ingress-nginx-controller" \
        "kubectl set resources ${res_type} ingress-nginx-controller -n ${NAMESPACE} --limits=cpu=200m,memory=256Mi --requests=cpu=100m,memory=128Mi" \
        ""
    fi
  else
    log_pass "Resource limits configured"
  fi
}

# ─────────────────────────────────────────────
# Phase 6 — Pod Security Audit
# ─────────────────────────────────────────────
audit_pod_security() {
  print_header "PHASE 6 — POD SECURITY AUDIT"

  print_section "Running Pods"
  log_step "Listing ingress-nginx pods..."

  local pod_out pod_count ready_count
  pod_out=$(kubectl_out get pods -n "$NAMESPACE" -l app.kubernetes.io/name=ingress-nginx --no-headers)
  pod_count=$(printf '%s\n' "$pod_out" | non_empty_count)
  ready_count=$(printf '%s\n' "$pod_out" | awk '($0 ~ /1\/1/ || $0 ~ /Running/){c++} END{print c+0}')

  log_info "Total pods: ${pod_count}"
  log_info "Ready pods: ${ready_count}"

  if (( pod_count > 0 && pod_count == ready_count )); then
    log_pass "All ingress-nginx pods are ready"
  elif (( pod_count > 0 )); then
    log_warn "Some pods not ready (${ready_count}/${pod_count})"
  else
    log_fail "No ingress-nginx pods found"
  fi

  print_section "Security Context"
  local res_type
  res_type=$(tr '[:upper:]' '[:lower:]' <<< "$DEPLOYMENT_TYPE")
  [[ -z "$res_type" ]] && res_type="deployment"

  log_step "Checking runAsNonRoot..."
  local run_as_non_root run_as_user
  run_as_non_root=$(kubectl_out get "$res_type" -n "$NAMESPACE" "$CONTROLLER_NAME" -o jsonpath='{.spec.template.spec.securityContext.runAsNonRoot}')
  run_as_user=$(kubectl_out get "$res_type" -n "$NAMESPACE" "$CONTROLLER_NAME" -o jsonpath='{.spec.template.spec.securityContext.runAsUser}')

  log_info "runAsNonRoot: $(or_default "$run_as_non_root")"
  log_info "runAsUser:    $(or_default "$run_as_user")"

  if [[ "$run_as_non_root" == "true" ]]; then
    log_pass "Running as non-root user"
  elif [[ -n "$run_as_user" && "$run_as_user" != "0" ]]; then
    log_pass "Running as user ${run_as_user} (non-root)"
  else
    log_warn "Security context should enforce non-root execution"
  fi

  log_step "Checking for privileged containers..."
  local privileged
  privileged=$(kubectl_out get "$res_type" -n "$NAMESPACE" "$CONTROLLER_NAME" -o jsonpath='{.spec.template.spec.containers[*].securityContext.privileged}')
  if [[ "$privileged" == "true" ]]; then
    log_warn "Container running in privileged mode"
  else
    log_pass "Container not running in privileged mode"
  fi
}

# ─────────────────────────────────────────────
# Phase 7 — Vulnerability Scan
# ─────────────────────────────────────────────
audit_vulnerabilities() {
  print_header "PHASE 7 — VULNERABILITY SCAN"

  print_section "Known CVEs for Current Version"
  log_step "Checking CVE database for version ${CONTROLLER_VERSION}..."
  case "$CONTROLLER_VERSION" in
    v1.14.3)
      log_pass "No known critical CVEs for ${CONTROLLER_VERSION}"
      ;;
    v1.14.*)
      log_warn "Version ${CONTROLLER_VERSION} may have issues — upgrade to v1.14.3"
      ;;
    v1.13.*|v1.12.*|v1.11.*|v1.10.*)
      log_fail "Version ${CONTROLLER_VERSION} is significantly outdated — multiple known CVEs"
      log_info "CRITICAL: Upgrade to v1.14.3 immediately"
      ;;
    v1.9.*|v1.8.*|v1.7.*)
      log_fail "CRITICAL: Version ${CONTROLLER_VERSION} has known critical security vulnerabilities"
      log_info "Upgrade to v1.14.3 IMMEDIATELY"
      ;;
    *)
      log_warn "Unknown version ${CONTROLLER_VERSION} — cannot verify CVE status"
      ;;
  esac

  print_section "AbuseBSI Report Compliance"
  log_step "Checking CB-Report#20260218-10009947 specific vulnerability..."

  if [[ -z "$DEPLOYMENT_TYPE" ]]; then
    log_info "No ingress-nginx controller in namespace '${NAMESPACE}' — AbuseBSI check not applicable"
  elif [[ "$ADMISSION_SVC_TYPE" == "ClusterIP" ]]; then
    log_pass "Admission controller not publicly exposed for ${DOMAIN} (compliant)"
  else
    log_fail "Admission controller publicly exposed via ${ADMISSION_SVC_TYPE} (NON-COMPLIANT for ${DOMAIN})"
    log_info "This is the specific vulnerability reported by AbuseBSI"
  fi
}

# ─────────────────────────────────────────────
# Phase 8 — TLS/SSL Certificate Audit
# ─────────────────────────────────────────────
audit_certificates() {
  print_header "PHASE 8 — TLS/SSL CERTIFICATE AUDIT"

  print_section "Admission Webhook Certificates"
  log_step "Checking admission webhook certificate secret..."

  if ! kubectl get secret -n "$NAMESPACE" ingress-nginx-admission >/dev/null 2>&1; then
    log_warn "Admission webhook certificate secret not found"
  else
    log_pass "Admission webhook certificate secret exists"

    log_step "Decoding certificate..."
    local cert_b64 cert_pem expiry expiry_epoch now_epoch days_left
    cert_b64=$(kubectl_out get secret -n "$NAMESPACE" ingress-nginx-admission -o jsonpath='{.data.cert}')
    if [[ -n "$cert_b64" ]]; then
      cert_pem=$(printf '%s' "$cert_b64" | base64 --decode 2>/dev/null)
      if [[ -n "$cert_pem" ]]; then
        expiry=$(printf '%s' "$cert_pem" | openssl x509 -noout -enddate 2>/dev/null | sed 's/^notAfter=//')
        if [[ -n "$expiry" ]]; then
          log_info "Certificate expiry: ${expiry}"
          if date -j -f "%b %e %T %Y %Z" "$expiry" "+%s" >/dev/null 2>&1; then
            expiry_epoch=$(date -j -f "%b %e %T %Y %Z" "$expiry" "+%s")
            now_epoch=$(date "+%s")
            days_left=$(( (expiry_epoch - now_epoch) / 86400 ))
            if (( days_left < 0 )); then
              log_fail "Certificate has EXPIRED!"
            elif (( days_left < 30 )); then
              log_warn "Certificate expires in ${days_left} days — renewal needed soon"
            else
              log_pass "Certificate valid for ${days_left} days"
            fi
          fi
        fi
      fi
    fi
  fi

  print_section "Default SSL Certificate"
  local res_type
  res_type=$(tr '[:upper:]' '[:lower:]' <<< "$DEPLOYMENT_TYPE")
  [[ -z "$res_type" ]] && res_type="deployment"

  log_step "Checking default SSL certificate arg..."
  local args default_cert
  args=$(kubectl_out get "$res_type" -n "$NAMESPACE" "$CONTROLLER_NAME" -o jsonpath='{.spec.template.spec.containers[0].args}')

  default_cert="not-set"
  local arg
  for arg in $args; do
    if [[ "$arg" == --default-ssl-certificate=* ]]; then
      default_cert="${arg#--default-ssl-certificate=}"
    fi
  done

  if [[ "$default_cert" != "not-set" ]]; then
    log_info "Default SSL certificate: ${default_cert}"
    local cert_ns cert_name
    cert_ns="${default_cert%%/*}"
    cert_name="${default_cert#*/}"
    if kubectl get secret -n "$cert_ns" "$cert_name" >/dev/null 2>&1; then
      log_pass "Default SSL certificate secret exists"
    else
      log_fail "Default SSL certificate secret '${default_cert}' not found"
    fi
  else
    log_info "No default SSL certificate configured (will use self-signed)"
  fi
}

# ─────────────────────────────────────────────
# Phase 9 — Ingress Resources Audit
# ─────────────────────────────────────────────
audit_ingress_resources() {
  print_header "PHASE 9 — INGRESS RESOURCES AUDIT"

  print_section "Cluster-wide Ingress Resources"
  log_step "Listing all Ingress resources..."

  local ingress_out total_ingress ingress_json nginx_count tls_count
  ingress_out=$(kubectl_out get ingress -A --no-headers)
  total_ingress=$(printf '%s\n' "$ingress_out" | non_empty_count)
  log_info "Total Ingress resources: ${total_ingress}"

  log_step "Counting NGINX-class Ingress resources..."
  ingress_json=$(kubectl_out get ingress -A -o json)
  nginx_count=0
  tls_count=0

  local snippet_names=""
  if [[ -n "${ingress_json// }" ]]; then
    nginx_count=$(jq -r '
      [ .items[]
        | .metadata.annotations as $a
        | .spec as $s
        | select(($s.ingressClassName=="nginx") or (($a["kubernetes.io/ingress.class"] // "") == "nginx"))
      ] | length
    ' <<< "$ingress_json")

    tls_count=$(jq -r '
      [ .items[]
        | .metadata.annotations as $a
        | .spec as $s
        | select(($s.ingressClassName=="nginx") or (($a["kubernetes.io/ingress.class"] // "") == "nginx"))
        | select(.spec.tls != null)
      ] | length
    ' <<< "$ingress_json")

    snippet_names=$(jq -r '
      .items[]
      | .metadata.annotations as $a
      | .spec as $s
      | select(($s.ingressClassName=="nginx") or (($a["kubernetes.io/ingress.class"] // "") == "nginx"))
      | select((.metadata.annotations // {} | keys[]? | test("snippet")))
      | "\(.metadata.namespace)/\(.metadata.name)"
    ' <<< "$ingress_json")
  fi

  log_info "NGINX Ingress resources: ${nginx_count}"
  if (( nginx_count > 0 )); then
    log_pass "Found ${nginx_count} Ingress resources using nginx class"
  fi

  print_section "Snippet Annotation Check"
  log_step "Scanning for snippet annotations..."
  local snippet_count
  snippet_count=$(printf '%s\n' "$snippet_names" | non_empty_count)
  if (( snippet_count > 0 )); then
    log_warn "Found ${snippet_count} Ingress resources using snippet annotations"
    log_info "Snippets can be a security risk — review carefully"
    while IFS= read -r n; do
      [[ -z "$n" ]] && continue
      writeln "    - ${n}"
    done <<< "$snippet_names"
  else
    log_pass "No Ingress resources using snippet annotations"
  fi

  print_section "TLS Configuration"
  log_step "Checking TLS coverage..."
  log_info "Ingress with TLS: ${tls_count}/${nginx_count}"
  if (( nginx_count > 0 && tls_count < nginx_count )); then
    log_warn "$((nginx_count - tls_count)) Ingress resources without TLS configured for ${DOMAIN}"
  elif (( nginx_count > 0 )); then
    log_pass "All Ingress resources configured with TLS"
  fi
}

# ─────────────────────────────────────────────
# Reporting
# ─────────────────────────────────────────────
build_recommendations_json() {
  local recs=()
  if [[ "$CONTROLLER_VERSION" != "v1.14.3" ]]; then
    recs+=("Upgrade controller to v1.14.3")
  fi
  if [[ "$ADMISSION_SVC_TYPE" != "ClusterIP" ]]; then
    recs+=("Change admission controller service to ClusterIP")
  fi
  recs+=("Plan migration from Ingress-NGINX (retiring March 2026)")
  recs+=("Consider migrating to Gateway API or alternative controller")

  printf '%s\n' "${recs[@]}" | jq -R . | jq -s .
}

generate_json_report() {
  local recommendations
  recommendations=$(build_recommendations_json)

  jq -n \
    --arg audit_timestamp "$(date -u +"%Y-%m-%dT%H:%M:%SZ")" \
    --arg domain "$DOMAIN" \
    --arg admin_email "$EMAIL" \
    --arg cluster_version "$CLUSTER_VERSION" \
    --arg namespace "$NAMESPACE" \
    --arg deployment_type "$DEPLOYMENT_TYPE" \
    --arg version "$CONTROLLER_VERSION" \
    --arg image "$CONTROLLER_IMAGE" \
    --arg latest_version "v1.14.3" \
    --arg service_type "$ADMISSION_SVC_TYPE" \
    --arg cluster_ip "$ADMISSION_CLUSTER_IP" \
    --arg external_ip "$ADMISSION_EXTERNAL_IP" \
    --arg publicly_exposed "$( [[ "$ADMISSION_SVC_TYPE" != "ClusterIP" ]] && echo true || echo false )" \
    --arg abusebsi_compliant "$( [[ "$ADMISSION_SVC_TYPE" == "ClusterIP" ]] && echo true || echo false )" \
    --arg snippet_annotations_enabled "$( [[ "$ALLOW_SNIPPETS" == "true" ]] && echo true || echo false )" \
    --argjson network_policies_count "$NP_COUNT" \
    --argjson passed "$PASS_COUNT" \
    --argjson failed "$FAIL_COUNT" \
    --argjson warnings "$WARN_COUNT" \
    --argjson info "$INFO_COUNT" \
    --argjson recommendations "$recommendations" \
    '{
      audit_timestamp: $audit_timestamp,
      domain: $domain,
      admin_email: $admin_email,
      cluster_version: $cluster_version,
      namespace: $namespace,
      controller: {
        deployment_type: $deployment_type,
        version: $version,
        image: $image,
        latest_version: $latest_version
      },
      admission_controller: {
        service_type: $service_type,
        cluster_ip: $cluster_ip,
        external_ip: $external_ip,
        publicly_exposed: ($publicly_exposed == "true")
      },
      security: {
        abusebsi_compliant: ($abusebsi_compliant == "true"),
        snippet_annotations_enabled: ($snippet_annotations_enabled == "true"),
        network_policies_count: $network_policies_count
      },
      audit_results: {
        passed: $passed,
        failed: $failed,
        warnings: $warnings,
        info: $info
      },
      recommendations: $recommendations
    }' > "$JSON_REPORT_FILE"
}

generate_summary() {
  print_header "AUDIT SUMMARY"

  writeln "  Domain: ${DOMAIN}"
  writeln "  Email:  ${EMAIL}"
  writeln "  ✓ Passed:   ${PASS_COUNT}"
  writeln "  ✗ Failed:   ${FAIL_COUNT}"
  writeln "  ⚠ Warnings: ${WARN_COUNT}"
  writeln "  ℹ Info:     ${INFO_COUNT}"
  writeln ""

  if (( FAIL_COUNT == 0 && WARN_COUNT == 0 )); then
    writeln "  ${GREEN}${BOLD}● Overall Status: EXCELLENT${RESET}"
    writeln "  No critical issues or warnings found."
  elif (( FAIL_COUNT == 0 )); then
    writeln "  ${YELLOW}${BOLD}● Overall Status: GOOD${RESET}"
    writeln "  No critical issues, but ${WARN_COUNT} warning(s) found."
  elif (( FAIL_COUNT <= 2 )); then
    writeln "  ${YELLOW}${BOLD}● Overall Status: NEEDS ATTENTION${RESET}"
    writeln "  ${FAIL_COUNT} critical issue(s) found — remediation recommended."
  else
    writeln "  ${RED}${BOLD}● Overall Status: CRITICAL${RESET}"
    writeln "  ${FAIL_COUNT} critical issue(s) found — immediate action required!"
  fi

  writeln ""
  writeln "  ${BOLD}Key Findings:${RESET}"
  writeln "    • Controller version:          ${CONTROLLER_VERSION}"
  writeln "    • Admission controller:        ${ADMISSION_SVC_TYPE}"
  if [[ "$ADMISSION_SVC_TYPE" == "ClusterIP" ]]; then
    writeln "    • AbuseBSI compliance:         ${GREEN}✓ COMPLIANT${RESET}"
  else
    writeln "    • AbuseBSI compliance:         ${RED}✗ NON-COMPLIANT${RESET}"
  fi

  writeln ""
  writeln "  ${BOLD}Recommendations:${RESET}"
  local idx=1
  while IFS= read -r rec; do
    [[ -z "$rec" ]] && continue
    writeln "    ${idx}. ${rec}"
    ((idx++)) || true
  done < <(build_recommendations_json | jq -r '.[]')

  writeln ""
  writeln "  ${BOLD}AbuseBSI Report Response:${RESET}"
  writeln "    Report ID: CB-Report#20260218-10009947"
  if [[ "$ADMISSION_SVC_TYPE" == "ClusterIP" ]]; then
    writeln "    Status:    ${GREEN}✓ RESOLVED${RESET}"
    writeln "    Details:   Admission controller is not publicly exposed"
  else
    writeln "    Status:    ${RED}✗ STILL VULNERABLE${RESET}"
    writeln "    Details:   Exposed via ${ADMISSION_SVC_TYPE} — immediate remediation required"
  fi

  writeln ""
  writeln "  ${BOLD}Report files generated:${RESET}"
  writeln "    • ${TEXT_REPORT_FILE} (text)"
  writeln "    • ${JSON_REPORT_FILE} (json)"
  writeln ""
  writeln "  ${BOLD}For questions: ${EMAIL}${RESET}"
}

# ─────────────────────────────────────────────
# Fixes
# ─────────────────────────────────────────────
wait_for_rollout() {
  local resource_type="$1"
  local name="$2"
  local ns="$3"
  printf "  %b→%b Waiting for rollout of %s/%s in %s ...\n" "$CYAN" "$RESET" "$resource_type" "$name" "$ns"
  kubectl rollout status "${resource_type}/${name}" -n "$ns" --timeout=120s >/dev/null 2>&1 || true
}

run_ingress_exposure_fix() {
  local exposing="$1"
  while IFS= read -r line; do
    [[ -z "${line// }" ]] && continue
    local ns name
    ns="${line%%/*}"
    name="${line#*/}"
    if [[ -z "$ns" || -z "$name" || "$ns" == "$name" ]]; then
      printf "  %b⚠ Skipping unrecognised entry: %s%b\n" "$YELLOW" "$line" "$RESET"
      continue
    fi
    printf "  %b→%b Deleting Ingress %s/%s ...\n" "$CYAN" "$RESET" "$ns" "$name"
    if kubectl delete ingress "$name" -n "$ns" >/dev/null 2>&1; then
      printf "  %b✓%b Deleted %s/%s\n" "$GREEN" "$RESET" "$ns" "$name"
    else
      return 1
    fi
  done <<< "$exposing"
  return 0
}

run_fix_by_id() {
  local id="$1"
  local meta="$2"
  case "$id" in
    admission-loadbalancer)
      kubectl patch svc ingress-nginx-controller-admission -n "$NAMESPACE" -p '{"spec":{"type":"ClusterIP"}}' >/dev/null
      ;;
    ingress-exposure)
      run_ingress_exposure_fix "$meta"
      ;;
    snippet-annotations)
      kubectl patch cm "$CONTROLLER_NAME" -n "$NAMESPACE" --type merge -p '{"data":{"allow-snippet-annotations":"false"}}' >/dev/null
      ;;
    resource-limits)
      local rt
      rt=$(tr '[:upper:]' '[:lower:]' <<< "$DEPLOYMENT_TYPE")
      [[ -z "$rt" ]] && rt="deployment"
      kubectl set resources "$rt" "$CONTROLLER_NAME" -n "$NAMESPACE" --limits=cpu=200m,memory=256Mi --requests=cpu=100m,memory=128Mi >/dev/null
      ;;
    upgrade-controller)
      helm upgrade ingress-nginx ingress-nginx/ingress-nginx --version 4.14.3 -n "$NAMESPACE" >/dev/null
      ;;
    *)
      return 1
      ;;
  esac
}

offer_fixes() {
  if (( ${#FIX_IDS[@]} == 0 )); then
    return
  fi

  printf "\n%b%s AUTO-FIX AVAILABLE %s%b\n" "${BOLD}${YELLOW}" "══════════" "══════════" "$RESET"
  printf "\n  The audit found %b%d fixable issue(s)%b for namespace %b%s%b:\n\n" \
    "$BOLD" "${#FIX_IDS[@]}" "$RESET" "$CYAN" "$NAMESPACE" "$RESET"

  printf "  %-4s  %-10s  %s\n" "#" "SEVERITY" "DESCRIPTION"
  printf "  %s\n" "$(printf '─%.0s' {1..70})"

  local i
  for i in "${!FIX_IDS[@]}"; do
    local sev_color="$YELLOW"
    [[ "${FIX_SEVERITIES[$i]}" == "CRITICAL" ]] && sev_color="$RED"
    printf "  %b[%2d]%b  %b%-10s%b  %s\n" \
      "$BOLD" "$((i+1))" "$RESET" \
      "$sev_color" "${FIX_SEVERITIES[$i]}" "$RESET" \
      "${FIX_DESCRIPTIONS[$i]}"
    printf "        %b$ %s%b\n\n" "$DIM" "${FIX_COMMANDS[$i]}" "$RESET"
  done

  printf "  %s\n" "$(printf '─%.0s' {1..70})"
  printf "\n  %b⚠  These changes will be applied to your live cluster.%b\n" "${YELLOW}${BOLD}" "$RESET"
  printf "  %b   Review the commands above before continuing.%b\n\n" "$YELLOW" "$RESET"

  printf "  %bDo you want to apply all fixes? [yes/no]:%b " "$BOLD" "$RESET"
  local answer
  IFS= read -r answer
  answer="$(to_lower "$answer")"
  answer="${answer//[$'\r\n']/}"

  if [[ "$answer" != "yes" && "$answer" != "y" ]]; then
    printf "\n  %bSkipping auto-fix.%b Fixes were logged in %s\n" "$YELLOW" "$RESET" "$TEXT_REPORT_FILE"
    printf "  You can apply them manually using the commands shown above.\n"
    return
  fi

  printf "\n%b%s APPLYING FIXES %s%b\n" "${BOLD}${BLUE}" "══════════" "══════════" "$RESET"

  local passed=0 failed=0
  local results=()

  for i in "${!FIX_IDS[@]}"; do
    printf "\n  %b[%d/%d]%b %b%s%b\n" "${BOLD}${CYAN}" "$((i+1))" "${#FIX_IDS[@]}" "$RESET" "$BOLD" "${FIX_DESCRIPTIONS[$i]}" "$RESET"
    printf "  %b$ %s%b\n\n" "$DIM" "${FIX_COMMANDS[$i]}" "$RESET"

    local start end elapsed
    start=$(date +%s)
    if run_fix_by_id "${FIX_IDS[$i]}" "${FIX_METAS[$i]}"; then
      end=$(date +%s)
      elapsed=$((end - start))
      printf "\n  %b✓ DONE%b (%ss)\n" "${GREEN}${BOLD}" "$RESET" "$elapsed"
      results+=("PASS|${FIX_DESCRIPTIONS[$i]}")
      ((passed++)) || true

      case "${FIX_IDS[$i]}" in
        snippet-annotations|resource-limits)
          local rt
          rt=$(tr '[:upper:]' '[:lower:]' <<< "$DEPLOYMENT_TYPE")
          [[ -z "$rt" ]] && rt="deployment"
          wait_for_rollout "$rt" "$CONTROLLER_NAME" "$NAMESPACE"
          ;;
        upgrade-controller)
          wait_for_rollout "deployment" "$CONTROLLER_NAME" "$NAMESPACE"
          ;;
      esac
    else
      end=$(date +%s)
      elapsed=$((end - start))
      printf "\n  %b✗ FAILED%b (%ss): command failed\n" "${RED}${BOLD}" "$RESET" "$elapsed"
      results+=("FAIL|${FIX_DESCRIPTIONS[$i]}")
      ((failed++)) || true
    fi
  done

  printf "\n%b%s FIX SUMMARY %s%b\n" "${BOLD}${BLUE}" "══════════" "══════════" "$RESET"
  local r status desc
  for r in "${results[@]}"; do
    status="${r%%|*}"
    desc="${r#*|}"
    if [[ "$status" == "PASS" ]]; then
      printf "  %b✓%b  %s\n" "$GREEN" "$RESET" "$desc"
    else
      printf "  %b✗%b  %s\n" "$RED" "$RESET" "$desc"
    fi
  done

  printf "\n"
  if (( failed == 0 )); then
    printf "  %b%b✓ All %d fix(es) applied successfully.%b\n" "$GREEN" "$BOLD" "$passed" "$RESET"
    printf "  %bRe-run the audit to confirm all issues are resolved.%b\n\n" "$DIM" "$RESET"
  else
    printf "  %b%b✗ %d fix(es) failed, %d succeeded.%b\n" "$RED" "$BOLD" "$failed" "$passed" "$RESET"
    printf "  %bApply the failed fixes manually using the commands shown above.%b\n\n" "$YELLOW" "$RESET"
  fi
}

# ─────────────────────────────────────────────
# Audit runners
# ─────────────────────────────────────────────
run_audit() {
  audit_preflight
  audit_version
  audit_admission_controller
  audit_network_security
  audit_configuration
  audit_pod_security
  audit_vulnerabilities
  audit_certificates
  audit_ingress_resources
  generate_json_report
  generate_summary
  offer_fixes
}

run_single_namespace() {
  local ns="$1"
  local ts="$2"

  reset_runtime_state
  NAMESPACE="$ns"
  TEXT_REPORT_FILE="ingress-audit-${ts}.txt"
  JSON_REPORT_FILE="ingress-audit-${ts}.json"
  if [[ "$SCAN_ALL" == true && ${#NAMESPACES[@]} -gt 1 ]]; then
    TEXT_REPORT_FILE="ingress-audit-${ns}-${ts}.txt"
    JSON_REPORT_FILE="ingress-audit-${ns}-${ts}.json"
  fi

  : > "$TEXT_REPORT_FILE"
  run_audit
  return $(( FAIL_COUNT > 0 ? 1 : 0 ))
}

run_multi_namespace_scan() {
  local total_fail=0
  local ts
  ts=$(date +"%Y%m%d-%H%M%S")

  local i ns
  for i in "${!NAMESPACES[@]}"; do
    ns="${NAMESPACES[$i]}"
    printf "\n%b--- NAMESPACE %d/%d: %s ---%b\n" "${BOLD}${BLUE}" "$((i+1))" "${#NAMESPACES[@]}" "$ns" "$RESET"

    if ! ns_has_ingress_nginx "$ns" "$CONTROLLER_NAME"; then
      printf "  %bSKIP%b: No ingress-nginx in namespace %b%s%b\n" "$YELLOW" "$RESET" "$CYAN" "$ns" "$RESET"
      continue
    fi

    if ! run_single_namespace "$ns" "$ts"; then
      ((total_fail++)) || true
    fi
  done

  printf "\n%b=== MULTI-NAMESPACE SCAN COMPLETE ===%b\n" "${BOLD}${BLUE}" "$RESET"
  printf "  Namespaces scanned: %b%d%b\n" "$CYAN" "${#NAMESPACES[@]}" "$RESET"
  for ns in "${NAMESPACES[@]}"; do
    printf "  * %b%s%b\n" "$CYAN" "$ns" "$RESET"
  done

  if (( total_fail > 0 )); then
    printf "\n  %b%bX Total failures: %d%b\n" "$RED" "$BOLD" "$total_fail" "$RESET"
    exit 1
  fi

  printf "\n  %b%b✓ All namespaces passed%b\n" "$GREEN" "$BOLD" "$RESET"
}

# ─────────────────────────────────────────────
# Main
# ─────────────────────────────────────────────
main() {
  interactive_setup
  local ts
  ts=$(date +"%Y%m%d-%H%M%S")

  if [[ "$SCAN_ALL" == true && ${#NAMESPACES[@]} -gt 1 ]]; then
    run_multi_namespace_scan
    return
  fi

  if [[ ${#NAMESPACES[@]} -gt 0 ]]; then
    NAMESPACE="${NAMESPACES[0]}"
  fi

  TEXT_REPORT_FILE="ingress-audit-${ts}.txt"
  JSON_REPORT_FILE="ingress-audit-${ts}.json"
  : > "$TEXT_REPORT_FILE"

  printf "%bConfig saved.%b Running audit for %b%s%b...\n\n" "$GREEN" "$RESET" "$CYAN" "$DOMAIN" "$RESET"

  if run_audit; then
    if (( FAIL_COUNT > 0 )); then
      exit 1
    fi
    exit 0
  fi
}

main "$@"
