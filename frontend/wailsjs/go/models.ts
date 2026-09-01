export namespace bridge {
	
	export class ConnectionConfig {
	    name: string;
	    host: string;
	    port: number;
	    version: string;
	    vin: string;
	    iccid: string;
	    subsystemCodes: string[];
	    heartbeatSec: number;
	    autoClockSync: boolean;
	    autoReconnect: boolean;
	    reportInterval: number;
	    reissueOffsetSec: number;
	    extensionPack?: string;
	    tls: tlsconf.Config;
	
	    static createFrom(source: any = {}) {
	        return new ConnectionConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.host = source["host"];
	        this.port = source["port"];
	        this.version = source["version"];
	        this.vin = source["vin"];
	        this.iccid = source["iccid"];
	        this.subsystemCodes = source["subsystemCodes"];
	        this.heartbeatSec = source["heartbeatSec"];
	        this.autoClockSync = source["autoClockSync"];
	        this.autoReconnect = source["autoReconnect"];
	        this.reportInterval = source["reportInterval"];
	        this.reissueOffsetSec = source["reissueOffsetSec"];
	        this.extensionPack = source["extensionPack"];
	        this.tls = this.convertValues(source["tls"], tlsconf.Config);
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
	export class ExportResult {
	    path: string;
	    count: number;
	
	    static createFrom(source: any = {}) {
	        return new ExportResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.count = source["count"];
	    }
	}
	export class PackInfo {
	    id: string;
	    label: string;
	    vendor?: string;
	    baseVersion: string;
	    unitCount: number;
	
	    static createFrom(source: any = {}) {
	        return new PackInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.label = source["label"];
	        this.vendor = source["vendor"];
	        this.baseVersion = source["baseVersion"];
	        this.unitCount = source["unitCount"];
	    }
	}
	export class PreviewResult {
	    cmd: string;
	    hex: string;
	    len: number;
	
	    static createFrom(source: any = {}) {
	        return new PreviewResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.cmd = source["cmd"];
	        this.hex = source["hex"];
	        this.len = source["len"];
	    }
	}
	export class ProfileSummary {
	    name: string;
	    host: string;
	    port: number;
	    active: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ProfileSummary(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.host = source["host"];
	        this.port = source["port"];
	        this.active = source["active"];
	    }
	}
	export class TestResult {
	    ok: boolean;
	    elapsedMs: number;
	    message: string;
	
	    static createFrom(source: any = {}) {
	        return new TestResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ok = source["ok"];
	        this.elapsedMs = source["elapsedMs"];
	        this.message = source["message"];
	    }
	}

}

export namespace engine {
	
	export class ParamResponseRow {
	    id: number;
	    hex: string;
	
	    static createFrom(source: any = {}) {
	        return new ParamResponseRow(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.hex = source["hex"];
	    }
	}

}

export namespace parser {
	
	export class Field {
	    offset: number;
	    length: number;
	    name: string;
	    type: string;
	    rawHex: string;
	    rawValue: string;
	    offsetVal: string;
	    translate: string;
	    unit: string;
	    note?: string;
	
	    static createFrom(source: any = {}) {
	        return new Field(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.offset = source["offset"];
	        this.length = source["length"];
	        this.name = source["name"];
	        this.type = source["type"];
	        this.rawHex = source["rawHex"];
	        this.rawValue = source["rawValue"];
	        this.offsetVal = source["offsetVal"];
	        this.translate = source["translate"];
	        this.unit = source["unit"];
	        this.note = source["note"];
	    }
	}
	export class Result {
	    totalBytes: number;
	    version: string;
	    versionByte: string;
	    command: string;
	    responseType: string;
	    vin: string;
	    encryption: string;
	    payloadLen: number;
	    fields: Field[];
	    warnings: string[];
	
	    static createFrom(source: any = {}) {
	        return new Result(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.totalBytes = source["totalBytes"];
	        this.version = source["version"];
	        this.versionByte = source["versionByte"];
	        this.command = source["command"];
	        this.responseType = source["responseType"];
	        this.vin = source["vin"];
	        this.encryption = source["encryption"];
	        this.payloadLen = source["payloadLen"];
	        this.fields = this.convertValues(source["fields"], Field);
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

}

export namespace schema {
	
	export class BitDef {
	    index: number;
	    label: string;
	
	    static createFrom(source: any = {}) {
	        return new BitDef(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.index = source["index"];
	        this.label = source["label"];
	    }
	}
	export class EnumDef {
	    value: number;
	    label: string;
	
	    static createFrom(source: any = {}) {
	        return new EnumDef(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.value = source["value"];
	        this.label = source["label"];
	    }
	}
	export class FieldSchema {
	    key: string;
	    label: string;
	    kind: string;
	    unit?: string;
	    min?: number;
	    max?: number;
	    enum?: EnumDef[];
	    bits?: BitDef[];
	    itemLabel?: string;
	    scaleNote?: string;
	    length?: number;
	
	    static createFrom(source: any = {}) {
	        return new FieldSchema(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.key = source["key"];
	        this.label = source["label"];
	        this.kind = source["kind"];
	        this.unit = source["unit"];
	        this.min = source["min"];
	        this.max = source["max"];
	        this.enum = this.convertValues(source["enum"], EnumDef);
	        this.bits = this.convertValues(source["bits"], BitDef);
	        this.itemLabel = source["itemLabel"];
	        this.scaleNote = source["scaleNote"];
	        this.length = source["length"];
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
	export class GroupSchema {
	    key: string;
	    title: string;
	    enabled: boolean;
	    multiple: boolean;
	    maxRows?: number;
	    fields: FieldSchema[];
	
	    static createFrom(source: any = {}) {
	        return new GroupSchema(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.key = source["key"];
	        this.title = source["title"];
	        this.enabled = source["enabled"];
	        this.multiple = source["multiple"];
	        this.maxRows = source["maxRows"];
	        this.fields = this.convertValues(source["fields"], FieldSchema);
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
	export class NamedGroup {
	    key: string;
	    enabled: boolean;
	    rows: any[];
	
	    static createFrom(source: any = {}) {
	        return new NamedGroup(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.key = source["key"];
	        this.enabled = source["enabled"];
	        this.rows = source["rows"];
	    }
	}
	export class GroupsPayload {
	    groups: NamedGroup[];
	
	    static createFrom(source: any = {}) {
	        return new GroupsPayload(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.groups = this.convertValues(source["groups"], NamedGroup);
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

export namespace tlsconf {
	
	export class Config {
	    enabled: boolean;
	    ca: string;
	    clientCert: string;
	    clientKey: string;
	    serverName: string;
	    insecure: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Config(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.enabled = source["enabled"];
	        this.ca = source["ca"];
	        this.clientCert = source["clientCert"];
	        this.clientKey = source["clientKey"];
	        this.serverName = source["serverName"];
	        this.insecure = source["insecure"];
	    }
	}

}

