// The Webhooks screen (owner only): endpoints that hear about what happens
// here, and how each delivery went. app.js calls Webhooks.init( me ) for admins;
// only the owner gets past the first line, and the server answers anyone else
// with a 404 regardless.
//
// Names, addresses and error text are other people's text, so every one goes
// through textContent. See the note at the top of dom.js.
const Webhooks = {
  open: null,

  async init( me ) {
    if ( !me.is_owner ) return;
    Dom.show( Dom.get( "webhooks-panel" ) , true );
    Dom.onSubmit( Dom.get( "create-webhook-form" ) , this.onCreate.bind( this ) );
    Dom.get( "copy-webhook-button" ).addEventListener( "click" , this.onCopy.bind( this ) );
    Dom.get( "webhook-all-events" ).addEventListener( "change" , this.syncEvents.bind( this ) );
    try {
      const catalogue = await Api.webhookEvents();
      this.renderEvents( catalogue.events );
    } catch ( error ) {
      App.showError( error.message );
    }
    await this.refresh();
  },

  // One checkbox per event. The names are the ones a receiver sees, so they
  // are shown as they are rather than translated.
  renderEvents( events ) {
    const holder = Dom.get( "webhook-events" );
    Dom.clear( holder );
    events.forEach( function ( event , index ) {
      const id = "webhook-event-" + index;
      holder.appendChild( Dom.el( "div" , { class: "form-check" , children: [
        Dom.el( "input" , { class: "form-check-input" , attrs: { id: id , type: "checkbox" , value: event } } ),
        Dom.el( "label" , { class: "form-check-label" , text: event , attrs: { for: id } } ),
      ] } ) );
    } );
    this.syncEvents();
  },

  syncEvents() {
    const all = Dom.get( "webhook-all-events" ).checked;
    Dom.show( Dom.get( "webhook-events" ) , !all );
  },

  pickedEvents() {
    if ( Dom.get( "webhook-all-events" ).checked ) return [ "*" ];
    return Array.from( Dom.get( "webhook-events" ).querySelectorAll( "input:checked" ) ).map( ( box ) => box.value );
  },

  async onCreate( event ) {
    event.preventDefault();
    const name = Dom.get( "webhook-name" ).value.trim();
    const url = Dom.get( "webhook-url" ).value.trim();
    try {
      const created = await Api.createWebhook( name , url , this.pickedEvents() );
      // Shown once: no route gives the signing secret out again.
      Dom.text( Dom.get( "webhook-secret-value" ) , created.secret );
      Dom.show( Dom.get( "webhook-secret-box" ) , true );
      Dom.get( "webhook-name" ).value = "";
      Dom.get( "webhook-url" ).value = "";
      await this.refresh();
    } catch ( error ) {
      App.showError( error.message );
    }
  },

  async onCopy() {
    await Dom.copy( Dom.get( "webhook-secret-value" ) );
  },

  button( key , tone , onClick ) {
    return Dom.el( "button" , {
      class: "btn btn-sm btn-outline-" + tone,
      text: I18n.get( key ),
      on: { click: onClick },
    } );
  },

  async refresh() {
    const body = Dom.get( "webhooks-table" ).querySelector( "tbody" );
    await Pager.load( "webhooks-table" , "webhooks" , ( offset ) => Api.webhooks( offset ) , ( hook ) => {
      const toggle = this.button( hook.enabled ? "webhooks.disable_button" : "webhooks.enable_button" , "secondary" , async () => {
        try {
          await Api.setWebhookEnabled( hook.id , !hook.enabled );
          await this.refresh();
        } catch ( error ) { App.showError( error.message ); }
      } );
      const test = this.button( "webhooks.test_button" , "secondary" , async () => {
        try {
          await Api.testWebhook( hook.id );
          await this.showDeliveries( hook );
          Dom.toast( I18n.get( "webhooks.test_sent" ) , "ok" );
        } catch ( error ) { App.showError( error.message ); }
      } );
      const history = this.button( "webhooks.deliveries_button" , "secondary" , () => this.showDeliveries( hook ) );
      // Delete asks nothing and offers Undo, like an invite link.
      const remove = this.button( "webhooks.delete_button" , "danger" , async () => {
        try {
          await Api.deleteWebhook( hook.id );
          if ( this.open === hook.id ) this.closeDeliveries();
          await this.refresh();
        } catch ( error ) {
          App.showError( error.message );
          return;
        }
        Undo.record( {
          label: I18n.get( "undo.deleted_webhook" ),
          toast: true,
          undo: async () => { await Api.restoreWebhook( hook.id ); await this.refresh(); },
          redo: async () => { await Api.deleteWebhook( hook.id ); await this.refresh(); },
        } );
      } );

      body.appendChild( Dom.el( "tr" , { children: [
        Dom.el( "td" , { text: hook.name } ),
        Dom.el( "td" , { class: "text-break" , text: hook.url } ),
        Dom.el( "td" , { children: [ hook.enabled
          ? Dom.badge( I18n.get( "webhooks.status_on" ) , "success" )
          : Dom.badge( I18n.get( "webhooks.status_off" ) , "secondary" ) ] } ),
        Dom.el( "td" , { children: [
          Dom.el( "div" , { class: "row-actions" , children: [ toggle , test , history , remove ] } ),
        ] } ),
      ] } ) );
    } );
  },

  closeDeliveries() {
    this.open = null;
    Dom.show( Dom.get( "webhook-deliveries" ) , false );
  },

  deliveryBadge( delivery ) {
    if ( delivery.status === "delivered" ) return Dom.badge( I18n.get( "webhooks.delivery_delivered" ) , "success" );
    if ( delivery.status === "failed" ) return Dom.badge( I18n.get( "webhooks.delivery_failed" ) , "danger" );
    return Dom.badge( I18n.get( "webhooks.delivery_pending" ) , "warning" );
  },

  async showDeliveries( hook ) {
    this.open = hook.id;
    Dom.text( Dom.get( "webhook-deliveries-heading" ) , I18n.get( "webhooks.deliveries_heading" ).replace( "{name}" , hook.name ) );
    Dom.show( Dom.get( "webhook-deliveries" ) , true );
    const body = Dom.get( "webhook-deliveries-table" ).querySelector( "tbody" );
    await Pager.load( "webhook-deliveries-table" , "deliveries" , ( offset ) => Api.webhookDeliveries( hook.id , offset ) , ( delivery ) => {
      body.appendChild( Dom.el( "tr" , { children: [
        Dom.el( "td" , { text: delivery.event } ),
        Dom.el( "td" , { children: [ this.deliveryBadge( delivery ) ] } ),
        Dom.el( "td" , { text: String( delivery.attempts ) } ),
        Dom.el( "td" , { class: "text-break" , text: delivery.last_error || ( delivery.last_status ? String( delivery.last_status ) : "" ) } ),
        Dom.el( "td" , { children: [ Dom.when( delivery.created_at ) ] } ),
        Dom.el( "td" , { children: delivery.status === "failed" ? [ this.resendButton( hook , delivery ) ] : [] } ),
      ] } ) );
    } );
  },

  // Once the receiver is fixed. The failed row stays, and a new one appears.
  resendButton( hook , delivery ) {
    return Dom.el( "button" , {
      class: "btn btn-sm btn-outline-secondary",
      text: I18n.get( "webhooks.resend_button" ),
      on: { click: async () => {
        try {
          await Api.resendDelivery( hook.id , delivery.id );
          Dom.toast( I18n.get( "webhooks.resent" ) , "ok" );
          await this.showDeliveries( hook );
        } catch ( error ) { App.showError( error.message ); }
      } },
    } );
  },
};
