import * as React from 'react';
import { WerftServiceClient } from './api/werft_pb_service';
import { JobStatus, GetJobRequest, GetJobResponse, LogSliceEvent, ListenRequest, ListenRequestLogs, JobPhase, StopJobRequest, StartFromPreviousJobRequest, SubscribeRequest, FilterExpression, FilterTerm, FilterOp, ListJobsRequest, OrderExpression } from './api/werft_pb';
import ReactTimeago from 'react-timeago';
import './components/terminal.css';
import { LogView } from './components/LogView';
import { ResultView } from './components/ResultView';
import { Header, headerStyles } from './components/header';
import { createStyles, Theme, Toolbar, Grid, Tooltip, IconButton, Tabs, Tab, Typography, Button, Snackbar, SnackbarContent } from '@material-ui/core';
import { WithStyles, withStyles } from '@material-ui/styles';
import CloseIcon from '@material-ui/icons/Close';
import StopIcon from '@material-ui/icons/Stop';
import ReplayIcon from '@material-ui/icons/Replay';
import InfoIcon from '@material-ui/icons/Info';
import { ColorUnknown, ColorFailure, ColorSuccess, ColorRunning } from './components/colors';
import { debounce, phaseToString } from './components/util';
import * as moment from 'moment';
import clsx from 'clsx';

const styles = (theme: Theme) => createStyles({
    main: {
        flex: 1,
        padding: theme.spacing(6, 4),
        background: '#eaeff1',
    },
    mainError: {
        flex: 1,
        padding: theme.spacing(6, 4),
        background: '#39355B'
    },
    errorMessage: {
        color: '#eaeff1',
        fontWeight: 800,
    },
    button: headerStyles(theme).button,
    metadataItemLabel: {
        fontWeight: "bold",
        paddingRight: "0.5em"
    },
    infobar: {
        paddingBottom: "1em"
    },
    newJobInfo: {
        backgroundColor: ColorRunning,
    },
    snackbarIcon: {
        fontSize: 20,
    },
    snackbarIconVariant: {
        opacity: 0.9,
        marginRight: theme.spacing(1),
    },
    snackbarMessage: {
        margin: '-8px 0px',
        display: 'flex',
        alignItems: 'center',
    },
    snackbarLink: {
        color: 'white'
    }
});

export interface JobViewProps extends WithStyles<typeof styles> {
    client: WerftServiceClient;
    jobName: string;
    view: "logs" | "raw-logs" | "results";
}

interface JobViewState {
    status?: JobStatus.AsObject
    showDetails: boolean;
    log: LogSliceEvent[];
    error?: any;
    newerJob?: JobStatus.AsObject
}

class JobViewImpl extends React.Component<JobViewProps, JobViewState> {
    protected logCache: LogSliceEvent[] = [];
    protected disposables: (()=>void)[] = [];

    constructor(props: JobViewProps) {
        super(props);
        this.state = {
            showDetails: true,
            log: []
        };
    }

    async componentDidMount() {
        window.addEventListener("keydown", returnToJobList);

        const req = new GetJobRequest();
        req.setName(this.props.jobName);
        try {
            const resp = await new Promise<GetJobResponse>((resolve, reject) => this.props.client.getJob(req, (err, resp) => !!err ? reject(err) : resolve(resp!)));
            const res = resp.getResult()!;
            this.setState({ status: res.toObject() });
        } catch (err) {
            this.setState({error: err});
            return;
        }
        
        this.listenForJobUpdates();
        this.listenForNewJobs();
    }

    protected listenForJobUpdates() {
        console.log("listening for updates to this job");
        
        const lreq = new ListenRequest();
        lreq.setLogs(ListenRequestLogs.LOGS_HTML);
        if (this.props.view === "raw-logs") {
            lreq.setLogs(ListenRequestLogs.LOGS_UNSLICED);
        }
        lreq.setUpdates(true);
        lreq.setName(this.props.jobName);

        try {
            const evts = this.props.client.listen(lreq);
            this.disposables.push(() => evts.cancel());
            
            let updateLogState = debounce((l: LogSliceEvent[]) => {
                this.setState({log: l});
            }, 200);
            evts.on('data', h => {
                if (h.hasUpdate()) {
                    this.setState({ status: h.getUpdate()!.toObject() });
                } else if (h.hasSlice()) {
                    const log = this.logCache;
                    log.push(h.getSlice()!);
                    updateLogState(log);
                }
            });
            evts.on('end', status => {
                if (!status) {
                    return;
                }
                // 0 === grpc.Code.OK
                if (status.code === 0) {
                    return;
                }

                setTimeout(() => this.listenForJobUpdates(), 1000);
            });
        } catch (err) {
            setTimeout(() => this.listenForJobUpdates(), 1000);
        }
    }

    protected listenForNewJobs() {
        console.log("listening for newer jobs");

        const nameTerm = new FilterTerm();
        nameTerm.setField("name");
        nameTerm.setOperation(FilterOp.OP_STARTS_WITH);
        nameTerm.setValue(this.props.jobName.split(".")[0]);
        const nufilter = new FilterExpression();
        nufilter.addTerms(nameTerm);

        const greq = new ListJobsRequest();
        greq.setFilterList([nufilter]);
        greq.setLimit(1);
        greq.setOrderList([ (() => {
            const e = new OrderExpression();
            e.setField("created");
            e.setAscending(false);
            return e;
        })() ]);
        this.props.client.listJobs(greq, (err, resp) => {
            if (!resp) {
                return;
            }
            const results = resp.getResultList();
            if (!results) {
                return;
            }
            const newJob = results[0];
            if (newJob.getName() === this.props.jobName) {
                return;
            }

            this.setState({ newerJob: newJob.toObject() });
        });

        try {
            const ureq = new SubscribeRequest();
            ureq.addFilter(nufilter);
            const newJobEvts = this.props.client.subscribe(ureq);
            this.disposables.push(() => newJobEvts.cancel());
            newJobEvts.on('data', h => {
                if (!h || !h.getResult()) {
                    return;
                }
                const r = h.getResult();
                if (!r) {
                    return;
                }
                if (r.getName() === this.props.jobName) {
                    return;
                }

                this.setState({ newerJob: r.toObject() });
            });
            newJobEvts.on('end', status => {
                if (!status) {
                    return;
                }
                // 0 === grpc.Code.OK
                if (status.code === 0) {
                    return;
                }

                setTimeout(() => this.listenForNewJobs(), 1000);
            });
        } catch (err) {
            setTimeout(() => this.listenForNewJobs(), 1000);
        }
    }

    componentWillUnmount() {
        window.removeEventListener("keydown", returnToJobList);
        this.disposables.forEach(d => d());
    }

    render() {
        let color = ColorUnknown;
        let failed = false;
        let finished = true;
        if (this.state.status && this.state.status.conditions) {
            if (this.state.status.phase !== JobPhase.PHASE_DONE) {
                color = ColorRunning;
                finished = false;
            } else if (this.state.status.conditions.success) {
                color = ColorSuccess;
            } else {
                color = ColorFailure;
                failed = true;
            }
        }

        const job = this.state.status;
        const actions = <React.Fragment>
            <Grid item xs></Grid>
            <Grid item>
                <Tabs onChange={() => {}} value={this.props.view}>
                    <Tab label="Logs" value="logs" href={`/job/${this.props.jobName}`} />
                    <Tab label="Raw Logs" value="raw-logs" href={`/job/${this.props.jobName}/raw`} />
                    { job && job.resultsList.length > 0 && <Tab label="Results" value="results" href={`/job/${this.props.jobName}/results`} /> }
                </Tabs>
            </Grid>
            <Grid item>
                { !!job && !![JobPhase.PHASE_PREPARING, JobPhase.PHASE_STARTING, JobPhase.PHASE_RUNNING].find(i => job.phase === i) && 
                    <Tooltip title="Cancel Job">
                        <IconButton color="inherit" onClick={() => this.stopJob()}>
                            <StopIcon />
                        </IconButton>
                    </Tooltip>
                }
                { !!job && job.phase === JobPhase.PHASE_DONE && job.conditions!.canReplay &&
                    <Tooltip title="Replay">
                        <IconButton color="inherit" onClick={() => this.replay()}>
                            <ReplayIcon />
                        </IconButton>
                    </Tooltip>
                }
                <Tooltip title="Back">
                    <IconButton color="inherit" onClick={() => window.location.href = "/jobs"}>
                        <CloseIcon />
                    </IconButton>
                </Tooltip>
            </Grid>
        </React.Fragment>

        const classes = this.props.classes;
        let secondary: React.ReactFragment | undefined;
        if (job) {
            secondary = <Toolbar>
                <Grid container spacing={1} alignItems="center" className={classes.infobar}>
                    <JobMetadataItemProps label="Owner">{job!.metadata!.owner}</JobMetadataItemProps>
                    <JobMetadataItemProps label="Repository">{`${job.metadata!.repository!.host}/${job.metadata!.repository!.owner}/${job.metadata!.repository!.repo}`}</JobMetadataItemProps>
                    <JobMetadataItemProps label="Ref">{job.metadata!.repository!.ref}</JobMetadataItemProps>
                    <JobMetadataItemProps label="Started"><ReactTimeago date={job.metadata!.created.seconds * 1000} /></JobMetadataItemProps>
                    <JobMetadataItemProps label="Revision" xs={6}>{job.metadata!.repository!.revision}</JobMetadataItemProps>
                    <JobMetadataItemProps label="Phase">{phaseToString(job.phase)}</JobMetadataItemProps>
                    { job.metadata!.finished && <JobMetadataItemProps label="Finished">
                        <Tooltip title={((job.metadata!.finished.seconds-job.metadata!.created.seconds)/60)+" minutes"}>
                            <span>
                                {moment.unix(job.metadata!.finished.seconds).from(moment.unix(job.metadata!.created.seconds))}
                            </span>
                        </Tooltip>
                    </JobMetadataItemProps> }
                </Grid>
            </Toolbar>;
        }

        const snackbar = (
            <Snackbar open={!!this.state.newerJob} anchorOrigin={{ vertical: 'bottom', horizontal: 'right' }}>
                <SnackbarContent 
                    className={classes.newJobInfo}
                    aria-describedby="client-snackbar"
                    message={
                        <span id="client-snackbar" className={classes.snackbarMessage}>
                            <InfoIcon className={clsx(classes.snackbarIcon, classes.snackbarIconVariant)} />
                            { this.state.newerJob && <p>This is not the latest job that ran with this context: <a className={classes.snackbarLink} href={`/job/${this.state.newerJob.name}`}>{this.state.newerJob.name}</a> is newer.</p> }
                        </span>
                    }
                    action={[
                        <IconButton key="close" aria-label="close" color="inherit" onClick={() => this.setState({ newerJob: undefined })}>
                            <CloseIcon className={classes.snackbarIcon} />
                        </IconButton>,
                    ]} />
            </Snackbar>
        );
        
        if (!!this.state.error) {
            return <main className={classes.mainError}>
                <Grid container alignContent="center" justify="center" direction="column">
                    <Grid item xs>
                        <Typography variant="h1" className={classes.errorMessage}>{this.state.error.metadata.headersMap["grpc-message"][0]}</Typography>
                        <Button variant="contained" href="/jobs">Back</Button>
                    </Grid>
                </Grid>
            </main>
        }

        return <React.Fragment>
            <Header color={color} title={this.props.jobName} actions={actions} secondary={secondary} />
            <main className={classes.main}>
                { snackbar }
                { this.state.status && this.state.status.details }
                { (this.props.view === "logs" || this.props.view === "raw-logs") &&
                    <LogView name={this.state.status && this.state.status.name} logs={this.state.log} failed={failed} raw={this.props.view === "raw-logs"} finished={finished} />
                }
                { this.props.view === "results" &&
                    <ResultView status={this.state.status} />  
                }
            </main> 
        </React.Fragment>
    }

    protected stopJob() {
        const req = new StopJobRequest();
        req.setName(this.props.jobName);
        this.props.client.stopJob(req, (err) => {
            if (!err) {
                return
            }

            alert(err);
        });
    }

    protected replay() {
        const job = this.state.status;
        if (!job) {
            return;
        }

        const req = new StartFromPreviousJobRequest();
        req.setPreviousJob(job.name);
        this.props.client.startFromPreviousJob(req, (err, ok) => {
            if (err) {
                alert(err);
                return;
            }

            window.location.href = "/job/" + ok!.getStatus()!.getName();
        });
    }

}

function returnToJobList(this: Window, evt: KeyboardEvent): any {
    if (evt.keyCode !== 27) {
        return
    }

    evt.preventDefault();
    window.location.replace("/jobs");
}

export const JobView = withStyles(styles)(JobViewImpl);

interface JobMetadataItemProps extends WithStyles<typeof styles> {
    label: string
    xs?: 1|2|3|4|5|6|7|8|9
}

class JobMetadataItemPropsImpl extends React.Component<JobMetadataItemProps, {}> {
    render() {
        return <Grid item xs={this.props.xs || 3}>
            <span className={this.props.classes.metadataItemLabel}>{this.props.label}</span>
            {this.props.children}
        </Grid>
    }
}

const JobMetadataItemProps = withStyles(styles)(JobMetadataItemPropsImpl)
