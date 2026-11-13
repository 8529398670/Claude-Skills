package apidoc

import (
	reflect "reflect"
	sort "sort"
	strconv "strconv"
	strings "strings"
	time "time"
)

// How a Go type becomes JSON Schema.
//
// string, bool, the ints and floats, slices, map[string]T and structs map
// over as you would expect; time.Time is a date-time string; any is any value;
// map[string]any is an object of anything. A pointer may also be null.
//
// A struct field's json tag names it, and it is required unless the tag says
// omitempty. The other tags describe it:
//
//	doc:"what it is"         its description
//	enum:"user|admin"        the values it may take
//	format:"date-time"       a string format
//	minimum:"0" maximum:"9"  number limits
//	minLength / maxLength    string limits
//	default / example        a value, as the field's type
//	nullable:"true"          may be null (a pointer already may)
//
// A type with an APIName method is a named schema in components, referred to
// by $ref. A type with an APIOneOf method is one of the types it lists, and
// one with an APIDoc method has that description. A struct embedding a named
// type is allOf that schema and its own fields; the methods it gets from the
// embedded type do not count as its own.

// Named types become components/schemas entries.
type Named interface{ APIName() string }

// Union types are one of several shapes.
type Union interface{ APIOneOf() []any }

// Described types say what they are.
type Described interface{ APIDoc() string }

var namedType = reflect.TypeOf( ( *Named )( nil ) ).Elem()
var unionType = reflect.TypeOf( ( *Union )( nil ) ).Elem()
var describedType = reflect.TypeOf( ( *Described )( nil ) ).Elem()

// declares is whether kind has the method of iface as its own, rather than
// promoted from a field it embeds.
func declares( kind reflect.Type , iface reflect.Type ) ( own bool ) {
	if kind.Implements( iface ) == false { return }
	own = true
	if kind.Kind() != reflect.Struct { return }
	method := iface.Method( 0 ).Name
	mine := reflect.Zero( kind ).MethodByName( method ).Call( nil )[ 0 ].Interface()
	for index := 0 ; index < kind.NumField() ; index++ {
		field := kind.Field( index )
		if field.Anonymous == false || field.Type.Implements( iface ) == false { continue }
		theirs := reflect.Zero( field.Type ).MethodByName( method ).Call( nil )[ 0 ].Interface()
		if reflect.DeepEqual( mine , theirs ) { own = false }
	}
	return
}
var timeType = reflect.TypeOf( time.Time{} )

type schemas struct {
	components map[string]any
	building   map[string]reflect.Type
}

func newSchemas() ( builder *schemas ) {
	builder = &schemas{ components: map[string]any{} , building: map[string]reflect.Type{} }
	return
}

// of is the schema for a type: a $ref for a named one.
func ( builder *schemas ) of( kind reflect.Type ) ( schema map[string]any ) {
	for kind.Kind() == reflect.Pointer { kind = kind.Elem() }
	if declares( kind , namedType ) {
		schema = builder.named( kind )
		return
	}
	schema = builder.inline( kind )
	return
}

func ( builder *schemas ) named( kind reflect.Type ) ( schema map[string]any ) {
	name := reflect.Zero( kind ).Interface().( Named ).APIName()
	schema = ref( name )
	if _ , started := builder.building[ name ]; started { return }
	builder.building[ name ] = kind
	builder.components[ name ] = builder.inline( kind )
	return
}

func ( builder *schemas ) inline( kind reflect.Type ) ( schema map[string]any ) {
	schema = builder.shape( kind )
	if declares( kind , describedType ) {
		schema[ "description" ] = reflect.Zero( kind ).Interface().( Described ).APIDoc()
	}
	return
}

func ( builder *schemas ) shape( kind reflect.Type ) ( schema map[string]any ) {
	if declares( kind , unionType ) {
		variants := []any{}
		for _ , variant := range reflect.Zero( kind ).Interface().( Union ).APIOneOf() {
			variants = append( variants , builder.of( reflect.TypeOf( variant ) ) )
		}
		schema = map[string]any{ "oneOf": variants }
		return
	}
	if kind == timeType {
		schema = map[string]any{ "type": "string" , "format": "date-time" }
		return
	}
	switch kind.Kind() {
	case reflect.String:
		schema = map[string]any{ "type": "string" }
	case reflect.Bool:
		schema = map[string]any{ "type": "boolean" }
	case reflect.Int , reflect.Int8 , reflect.Int16 , reflect.Int32 , reflect.Int64 , reflect.Uint , reflect.Uint8 , reflect.Uint16 , reflect.Uint32 , reflect.Uint64:
		schema = map[string]any{ "type": "integer" }
	case reflect.Float32 , reflect.Float64:
		schema = map[string]any{ "type": "number" }
	case reflect.Slice , reflect.Array:
		schema = map[string]any{ "type": "array" , "items": builder.of( kind.Elem() ) }
	case reflect.Map:
		schema = map[string]any{ "type": "object" }
		if kind.Elem().Kind() != reflect.Interface { schema[ "additionalProperties" ] = builder.of( kind.Elem() ) }
	case reflect.Struct:
		schema = builder.object( kind )
	default:
		schema = map[string]any{}
	}
	return
}

func ( builder *schemas ) object( kind reflect.Type ) ( schema map[string]any ) {
	parts := []any{}
	properties := map[string]any{}
	required := []any{}
	var add func( kind reflect.Type )
	add = func( kind reflect.Type ) {
		for index := 0 ; index < kind.NumField() ; index++ {
			field := kind.Field( index )
			if field.Anonymous {
				embedded := field.Type
				for embedded.Kind() == reflect.Pointer { embedded = embedded.Elem() }
				if declares( embedded , namedType ) {
					parts = append( parts , builder.named( embedded ) )
				} else {
					add( embedded )
				}
				continue
			}
			name , omit := jsonName( field )
			if name == "" { continue }
			properties[ name ] = builder.field( field , true )
			if omit == false { required = append( required , name ) }
		}
	}
	add( kind )
	own := map[string]any{ "type": "object" }
	if len( properties ) > 0 { own[ "properties" ] = properties }
	if len( required ) > 0 {
		sort.Slice( required , func( a , b int ) bool { return required[ a ].( string ) < required[ b ].( string ) } )
		own[ "required" ] = required
	}
	if len( parts ) == 0 {
		schema = own
		return
	}
	if len( properties ) > 0 { parts = append( parts , own ) }
	schema = map[string]any{ "allOf": parts }
	return
}

func jsonName( field reflect.StructField ) ( name string , omit bool ) {
	if field.IsExported() == false { return }
	tag := field.Tag.Get( "json" )
	if tag == "-" { return }
	pieces := strings.Split( tag , "," )
	name = pieces[ 0 ]
	if name == "" { name = field.Name }
	for _ , option := range pieces[ 1 : ] {
		if option == "omitempty" || option == "omitzero" { omit = true }
	}
	return
}

var scalarTypes = map[string]bool{ "string": true , "integer": true , "number": true , "boolean": true }

// field is a struct field's schema, with its tags applied. A parameter's
// doc tag describes the parameter rather than its schema, so with_doc is
// false for those.
func ( builder *schemas ) field( field reflect.StructField , with_doc bool ) ( schema map[string]any ) {
	base := builder.of( field.Type )
	schema = map[string]any{}
	for key , value := range base { schema[ key ] = value }
	kind := field.Type
	nullable := kind.Kind() == reflect.Pointer || field.Tag.Get( "nullable" ) == "true"
	for kind.Kind() == reflect.Pointer { kind = kind.Elem() }
	_ , is_ref := schema[ "$ref" ]
	_ , is_union := schema[ "oneOf" ]
	_ , is_all := schema[ "allOf" ]
	if is_ref == false && is_union == false && is_all == false {
		typed := typeOf( schema )
		if value := field.Tag.Get( "enum" ); value != "" {
			values := []any{}
			for _ , piece := range strings.Split( value , "|" ) { values = append( values , typedValue( piece , typed ) ) }
			schema[ "enum" ] = values
		}
		for _ , name := range []string{ "format" } {
			if value := field.Tag.Get( name ); value != "" { schema[ name ] = value }
		}
		for _ , name := range []string{ "minimum" , "maximum" , "minLength" , "maxLength" } {
			if value := field.Tag.Get( name ); value != "" { schema[ name ] = number( value ) }
		}
		for _ , name := range []string{ "default" , "example" } {
			if value , found := field.Tag.Lookup( name ); found { schema[ name ] = typedValue( value , typed ) }
		}
	}
	if nullable {
		if typed , ok := schema[ "type" ].( string ); ok && scalarTypes[ typed ] {
			schema[ "type" ] = []any{ typed , "null" }
		} else {
			schema = map[string]any{ "oneOf": []any{ schema , map[string]any{ "type": "null" } } }
		}
	}
	if doc := field.Tag.Get( "doc" ); doc != "" && with_doc { schema[ "description" ] = doc }
	return
}

func typeOf( schema map[string]any ) ( typed string ) {
	typed , _ = schema[ "type" ].( string )
	return
}

func number( text string ) ( value any ) {
	if whole , err := strconv.ParseInt( text , 10 , 64 ); err == nil {
		value = whole
		return
	}
	if real , err := strconv.ParseFloat( text , 64 ); err == nil {
		value = real
		return
	}
	value = text
	return
}

func typedValue( text string , typed string ) ( value any ) {
	switch typed {
	case "integer" , "number":
		value = number( text )
	case "boolean":
		value = text == "true"
	default:
		value = text
	}
	return
}
