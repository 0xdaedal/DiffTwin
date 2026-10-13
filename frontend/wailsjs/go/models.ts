export namespace comparison {

	export class Response {
	    status_code: number;
	    headers: Record<string, string>;
	    body: string;
	    duration_ms: number;
	    truncated: boolean;
	    error?: string;

	    static createFrom(source: any = {}) {
	        return new Response(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.status_code = source["status_code"];
	        this.headers = source["headers"];
	        this.body = source["body"];
	        this.duration_ms = source["duration_ms"];
	        this.truncated = source["truncated"];
	        this.error = source["error"];
	    }
	}
	export class Result {
	    primary: Response;
	    candidate: Response;
	    secondary: Response;
	    secondary_enabled: boolean;
	    candidate_differs: boolean;
	    noise_detected: boolean;

	    static createFrom(source: any = {}) {
	        return new Result(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.primary = this.convertValues(source["primary"], Response);
	        this.candidate = this.convertValues(source["candidate"], Response);
	        this.secondary = this.convertValues(source["secondary"], Response);
	        this.secondary_enabled = source["secondary_enabled"];
	        this.candidate_differs = source["candidate_differs"];
	        this.noise_detected = source["noise_detected"];
	    }

		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

export namespace domain {

	export class AuthCredentials {
	    token?: string;
	    username?: string;
	    password?: string;
	    api_key?: string;

	    static createFrom(source: any = {}) {
	        return new AuthCredentials(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.token = source["token"];
	        this.username = source["username"];
	        this.password = source["password"];
	        this.api_key = source["api_key"];
	    }
	}
	export class Folder {
	    id: string;
	    name: string;
	    requests?: Request[];
	    folders?: Folder[];

	    static createFrom(source: any = {}) {
	        return new Folder(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.requests = this.convertValues(source["requests"], Request);
	        this.folders = this.convertValues(source["folders"], Folder);
	    }

		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class Request {
	    id: string;
	    name: string;
	    method: string;
	    path: string;
	    headers?: Record<string, string>;
	    body?: string;

	    static createFrom(source: any = {}) {
	        return new Request(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.method = source["method"];
	        this.path = source["path"];
	        this.headers = source["headers"];
	        this.body = source["body"];
	    }
	}
	export class Collection {
	    id: string;
	    name: string;
	    requests: Request[];
	    folders?: Folder[];

	    static createFrom(source: any = {}) {
	        return new Collection(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.requests = this.convertValues(source["requests"], Request);
	        this.folders = this.convertValues(source["folders"], Folder);
	    }

		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class DestinationAuth {
	    type: string;
	    api_key_name?: string;
	    api_key_location?: string;

	    static createFrom(source: any = {}) {
	        return new DestinationAuth(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.type = source["type"];
	        this.api_key_name = source["api_key_name"];
	        this.api_key_location = source["api_key_location"];
	    }
	}

	export class Project {
	    id: string;
	    name: string;
	    primary: string;
	    candidate: string;
	    secondary: string;
	    auth?: Record<string, DestinationAuth>;
	    ignored_json_paths?: string[];
	    variables?: Record<string, string>;
	    destination_variables?: Record<string, any>;
	    collections: Collection[];

	    static createFrom(source: any = {}) {
	        return new Project(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.primary = source["primary"];
	        this.candidate = source["candidate"];
	        this.secondary = source["secondary"];
	        this.auth = this.convertValues(source["auth"], DestinationAuth, true);
	        this.ignored_json_paths = source["ignored_json_paths"];
	        this.variables = source["variables"];
	        this.destination_variables = source["destination_variables"];
	        this.collections = this.convertValues(source["collections"], Collection);
	    }

		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

export namespace main {

	export class PostmanImportResult {
	    collection: domain.Collection;
	    warnings: string[];

	    static createFrom(source: any = {}) {
	        return new PostmanImportResult(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.collection = this.convertValues(source["collection"], domain.Collection);
	        this.warnings = source["warnings"];
	    }

		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ProjectAuthSettings {
	    auth: Record<string, domain.DestinationAuth>;
	    credentials: Record<string, domain.AuthCredentials>;
	    credentials_missing: boolean;

	    static createFrom(source: any = {}) {
	        return new ProjectAuthSettings(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.auth = this.convertValues(source["auth"], domain.DestinationAuth, true);
	        this.credentials = this.convertValues(source["credentials"], domain.AuthCredentials, true);
	        this.credentials_missing = source["credentials_missing"];
	    }

		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}
