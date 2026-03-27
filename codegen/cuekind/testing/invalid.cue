package testing

// appName must match ^([a-z][a-z0-9-]*[a-z0-9])$
invalidAppNameUppercase: {
	appName: "BadApp"
	versions: {
		"v1": {
			kinds: [{
				kind: "Valid"
				schema: spec: field: string
			}]
		}
	}
}

// appName is a required field
invalidAppNameMissing: {
	versions: {
		"v1": {
			kinds: [{
				kind: "Valid"
				schema: spec: field: string
			}]
		}
	}
}

// kind must match ^([A-Z][a-zA-Z0-9-]{0,61}[a-zA-Z0-9])$
invalidKindNameLowercase: {
	appName: "bad-kind"
	versions: {
		"v1": {
			kinds: [{
				kind: "lowercasekind"
				schema: spec: field: string
			}]
		}
	}
}

// route name is a required field on #CustomRoute
invalidRouteNameMissing: {
	appName: "missing-route-name"
	versions: {
		"v1": {
			kinds: [{
				kind: "NoName"
				schema: spec: field: string
				routes: {
					"/noname": {
						"GET": {
							response: ok: bool
						}
					}
				}
			}]
		}
	}
}

// route name must match ^(get|log|read|replace|patch|delete|deletecollection|watch|connect|proxy|list|create|patch)([A-Za-z0-9]+)$
invalidRouteNameBadPrefix: {
	appName: "bad-route"
	versions: {
		"v1": {
			kinds: [{
				kind: "RouteKind"
				schema: spec: field: string
				routes: {
					"/broken": {
						"GET": {
							name: "badName"
							response: ok: bool
						}
					}
				}
			}]
		}
	}
}

// route method must be one of GET, POST, PUT, DELETE, PATCH, *
invalidRouteMethod: {
	appName: "bad-method"
	versions: {
		"v1": {
			kinds: [{
				kind: "MethodKind"
				schema: spec: field: string
				routes: {
					"/test": {
						"INVALID": {
							name: "getTest"
							response: ok: bool
						}
					}
				}
			}]
		}
	}
}

// scope must be "Cluster" or "Namespaced"
invalidScope: {
	appName: "bad-scope"
	versions: {
		"v1": {
			kinds: [{
				kind:  "ScopeKind"
				scope: "BadScope"
				schema: spec: field: string
			}]
		}
	}
}

// groupOverride must match ^([a-z][a-z0-9-.]{0,48}[a-z0-9])\.([a-z][a-z0-9-]{0,48}[a-z0-9])$
invalidGroupOverride: {
	appName:       "bad-group"
	groupOverride: "INVALID"
	versions: {
		"v1": {
			kinds: [{
				kind: "GroupKind"
				schema: spec: field: string
			}]
		}
	}
}

// version-level route name must also match the #CustomRoute regex
invalidVersionRouteName: {
	appName: "bad-version-route"
	versions: {
		"v1": {
			kinds: [{
				kind: "Vr"
				schema: spec: field: string
			}]
			routes: namespaced: {
				"/bad": {
					"POST": {
						name: "notAValidPrefix"
						response: ok: bool
					}
				}
			}
		}
	}
}

// version-level namespaced route with missing name
invalidNamespacedRouteMissingName: {
	appName: "bad-ns-route"
	versions: {
		"v1": {
			kinds: [{
				kind: "NsKind"
				schema: spec: field: string
			}]
			routes: namespaced: {
				"/noname": {
					"GET": {
						response: ok: bool
					}
				}
			}
		}
	}
}

// version-level cluster route with invalid name prefix
invalidClusterRouteBadName: {
	appName: "bad-cluster-route"
	versions: {
		"v1": {
			kinds: [{
				kind: "ClKind"
				schema: spec: field: string
			}]
			routes: cluster: {
				"/bad": {
					"POST": {
						name: "invalidPrefix"
						response: ok: bool
					}
				}
			}
		}
	}
}

// #CustomRoute.extensions keys must match ^x-(.+)$
invalidExtensionKey: {
	appName: "ext-app"
	versions: {
		"v1": {
			kinds: [{
				kind: "ExtKind"
				schema: spec: field: string
				routes: {
					"/ext": {
						"GET": {
							name: "getExt"
							response: ok: bool
							extensions: {
								"not-x-prefixed": true
							}
						}
					}
				}
			}]
		}
	}
}

// #AdditionalPrinterColumns requires name, type, and jsonPath
invalidPrinterColumnMissingFields: {
	appName: "printer-app"
	versions: {
		"v1": {
			kinds: [{
				kind: "PrintKind"
				schema: spec: field: string
				additionalPrinterColumns: [{
					name: "col"
				}]
			}]
		}
	}
}

// Manifest with no versions (empty map triggers list.Sort on empty _allVersions)
invalidEmptyVersions: {
	appName:  "no-versions"
	versions: {}
}

// #Role.title must be non-empty (string & !="")
invalidRoleEmptyTitle: {
	appName: "role-app"
	versions: {
		"v1": {
			kinds: [{
				kind: "RoleKind"
				schema: spec: field: string
			}]
		}
	}
	roles: {
		"role-app:reader": {
			title: ""
			kinds: [{
				kind: "RoleKind"
			}]
		}
	}
}

// pluralName must match ^([A-Z][a-zA-Z0-9-]{0,61}[a-zA-Z])$ if explicitly set
invalidPluralName: {
	appName: "plural-app"
	versions: {
		"v1": {
			kinds: [{
				kind:       "PluralKind"
				pluralName: "lowercase-bad!"
				schema: spec: field: string
			}]
		}
	}
}

